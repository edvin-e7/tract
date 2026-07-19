package extract

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
)

// permissiveFetcher keeps the dial seam (real client, real Control hook) but
// allows every address — for tests whose fixture servers live on loopback.
func permissiveFetcher() *Fetcher {
	return &Fetcher{Client: newHTTPClient(func(netip.Addr) bool { return true })}
}

// TestAddrAllowed pins the IP-classification contract. The falsifying half is
// the allowed list: a guard that just returns false would pass every "blocked"
// row and fail these.
func TestAddrAllowed(t *testing.T) {
	blocked := []string{
		"127.0.0.1",          // loopback
		"127.8.8.8",          // whole 127/8, not just .1
		"10.0.0.7",           // RFC 1918
		"172.16.0.1",         // RFC 1918
		"192.168.1.1",        // RFC 1918
		"169.254.169.254",    // link-local — cloud metadata endpoint
		"100.64.0.1",         // CGNAT / RFC 6598
		"0.0.0.0",            // unspecified
		"255.255.255.255",    // broadcast
		"224.0.0.251",        // multicast
		"::1",                // v6 loopback
		"fe80::1",            // v6 link-local
		"fdaa::3",            // ULA — Fly 6PN lives in fdaa::/16
		"::ffff:127.0.0.1",   // 4-in-6 mapped loopback (Unmap seam)
		"::ffff:192.168.0.9", // 4-in-6 mapped private
		"::",                 // v6 unspecified
	}
	for _, s := range blocked {
		if addrAllowed(netip.MustParseAddr(s)) {
			t.Errorf("addrAllowed(%s) = true, want blocked", s)
		}
	}

	allowed := []string{
		"93.184.216.34",        // example.com
		"8.8.8.8",              // public v4
		"1.1.1.1",              // public v4
		"2001:4860:4860::8888", // public v6
	}
	for _, s := range allowed {
		if !addrAllowed(netip.MustParseAddr(s)) {
			t.Errorf("addrAllowed(%s) = false, want allowed (guard over-blocks)", s)
		}
	}
}

// TestNewBlocksPrivateAddresses drives the REAL production fetcher (New) at a
// loopback server and a private-resolving hostname: the dial guard must refuse
// before a single request reaches the socket.
func TestNewBlocksPrivateAddresses(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("<html><body><p>should never be reached</p></body></html>"))
	}))
	defer srv.Close()

	for _, target := range []string{
		srv.URL,                   // literal 127.0.0.1:port
		"http://localhost:1/",     // DNS name resolving to loopback
		"http://169.254.169.254/", // metadata IP
		"http://192.168.1.1/x",    // RFC 1918
	} {
		_, err := New().Fetch(context.Background(), target)
		if err == nil {
			t.Fatalf("Fetch(%s) succeeded, want SSRF block", target)
		}
		if !strings.Contains(err.Error(), "blocked address") {
			t.Fatalf("Fetch(%s) error = %v, want a 'blocked address' dial refusal", target, err)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("fixture server received %d request(s); the guard must refuse before connecting", n)
	}
}

// TestRedirectToPrivateBlockedMidChain is the classic bypass: hop 1 is an
// allowed address, hop 2 redirects to a private one. Because the guard lives
// in the dialer's Control hook, hop 2's dial must fail even though hop 1
// passed. The allow predicate permits loopback only (so the loopback fixture
// can play the "public" first hop) while link-local stays blocked.
func TestRedirectToPrivateBlockedMidChain(t *testing.T) {
	var hits atomic.Int32
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer redirector.Close()

	f := &Fetcher{Client: newHTTPClient(func(a netip.Addr) bool { return a.Unmap().IsLoopback() })}
	_, err := f.Fetch(context.Background(), redirector.URL)
	if err == nil {
		t.Fatal("redirect to 169.254.169.254 succeeded, want mid-chain block")
	}
	if !strings.Contains(err.Error(), "blocked address 169.254.169.254") {
		t.Fatalf("error = %v, want a dial refusal naming 169.254.169.254", err)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("redirector hits = %d, want exactly 1 (redirect must be followed, then blocked at the next dial)", n)
	}
}

// TestFetchRejectsOversizedBody proves the OOM cap: a body over maxFetchBytes
// must produce an error, not a silently truncated stored article. Both paths
// are covered — the declared Content-Length fast reject and the streamed
// (chunked, no Content-Length) LimitedReader backstop.
func TestFetchRejectsOversizedBody(t *testing.T) {
	t.Run("declared content-length", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("Content-Length", fmt.Sprint(maxFetchBytes+1024))
			// Write nothing further; the fast path must reject on the header.
		}))
		defer srv.Close()

		_, err := permissiveFetcher().Fetch(context.Background(), srv.URL)
		if err == nil || !strings.Contains(err.Error(), "limit") {
			t.Fatalf("error = %v, want over-the-limit rejection from Content-Length", err)
		}
	})

	t.Run("streamed without content-length", func(t *testing.T) {
		chunk := []byte("<p>" + strings.Repeat("padding words for the reader ", 100) + "</p>\n")
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html><body>"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush() // force chunked encoding — no Content-Length
			}
			written := 0
			for written <= maxFetchBytes+len(chunk) {
				n, err := w.Write(chunk)
				if err != nil {
					return // client hung up (cap reached) — expected
				}
				written += n
			}
		}))
		defer srv.Close()

		_, err := permissiveFetcher().Fetch(context.Background(), srv.URL)
		if err == nil || !strings.Contains(err.Error(), "limit") {
			t.Fatalf("error = %v, want over-the-limit rejection from the capped reader", err)
		}
	})
}
