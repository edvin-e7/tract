// SSRF guard for the article fetcher. POST /api/items dials an arbitrary
// caller-supplied URL server-side, so the HTTP client must refuse to connect
// to private, loopback, link-local and otherwise non-global addresses — on a
// public deploy an unguarded fetcher is a proxy into the host's internal
// network (e.g. Fly's 6PN, cloud metadata endpoints).
//
// The check runs in the dialer's Control hook, i.e. against the ACTUAL
// resolved "ip:port" of every connection attempt. That single seam covers the
// three classic bypasses at once: literal private URLs, DNS names that resolve
// to private ranges (incl. rebinding — the check runs on the address being
// dialed, not a pre-flight lookup), and redirects that hop to a private
// address mid-chain (each hop is a fresh dial through the same hook).
package extract

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// cgnat is RFC 6598 shared address space (100.64.0.0/10) — non-global (carrier
// NAT, Tailscale), refused along with the standard private ranges.
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// addrAllowed reports whether the fetcher may connect to a. Only global
// unicast addresses pass; everything reserved for local/internal use is
// refused.
func addrAllowed(a netip.Addr) bool {
	a = a.Unmap() // ::ffff:127.0.0.1 must be judged as 127.0.0.1
	switch {
	case !a.IsValid(),
		a.IsLoopback(),                // 127.0.0.0/8, ::1
		a.IsPrivate(),                 // RFC 1918 + RFC 4193 ULA (fc00::/7 — Fly 6PN lives here)
		a.IsLinkLocalUnicast(),        // 169.254.0.0/16 (cloud metadata) + fe80::/10
		a.IsLinkLocalMulticast(),      //
		a.IsInterfaceLocalMulticast(), //
		a.IsMulticast(),               //
		a.IsUnspecified(),             // 0.0.0.0, ::
		cgnat.Contains(a),
		a == netip.AddrFrom4([4]byte{255, 255, 255, 255}): // IPv4 broadcast
		return false
	}
	return a.IsGlobalUnicast()
}

// newHTTPClient builds the fetch client with the dial-time address guard.
// allow decides per resolved address; production (New) passes addrAllowed,
// tests may inject a permissive predicate because unit-test servers live on
// loopback, which the production guard refuses by design.
func newHTTPClient(allow func(netip.Addr) bool) *http.Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		// Control receives the post-DNS "ip:port" for every connection
		// attempt — first hop and every redirect hop alike.
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("refusing to dial %q: %w", address, err)
			}
			ip, err := netip.ParseAddr(host)
			if err != nil {
				return fmt.Errorf("refusing to dial non-IP address %q", host)
			}
			if !allow(ip) {
				return fmt.Errorf("blocked address %s: private, loopback, link-local and metadata ranges are not fetchable", ip)
			}
			return nil
		},
	}
	return &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			// Deliberately no Proxy: routing fetches through a proxy would
			// bypass the dial-time IP guard (the guard would only ever see
			// the proxy's address).
			DialContext:         dialer.DialContext,
			ForceAttemptHTTP2:   true,
			MaxIdleConns:        10,
			IdleConnTimeout:     30 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}
}
