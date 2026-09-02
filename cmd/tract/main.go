// Command tract is the single-binary read-later server: it serves the JSON API
// and the embedded frontend build from one process.
package main

import (
	"embed"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/edvin-e7/tract/internal/api"
	"github.com/edvin-e7/tract/internal/extract"
	"github.com/edvin-e7/tract/internal/store"
)

// dist holds the built frontend. The committed placeholder index.html makes the
// embed valid before the first `npm run build`; a real build overwrites it.
//
//go:embed all:dist
var dist embed.FS

func main() {
	addr := resolveAddr()
	dbPath := envOr("TRACT_DB", "tract.db")

	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		log.Fatalf("embed: %v", err)
	}

	token := os.Getenv("TRACT_TOKEN")
	warnIfOpenToTheNetwork(addr, token)

	// TRACT_PRIVATE=1 also gates the READ routes, so a public hostname does not
	// hand the whole reading list to anyone who has the URL.
	private := envTruthy("TRACT_PRIVATE")
	if private && token == "" {
		// Refuse rather than start half-secured. Private with no token is a gate
		// with nothing to check: it would lock the owner out and protect nobody,
		// and — worse — it would print a reassuring "private mode" line while
		// every read stayed open.
		log.Fatalf("TRACT_PRIVATE is set but TRACT_TOKEN is empty — private mode cannot gate anything without a token. Generate one with: openssl rand -hex 32")
	}
	if private {
		log.Printf("private mode: read routes require a bearer token (GET / still serves the app shell)")
	}

	srv := &api.Server{
		Store:        st,
		Extractor:    extract.New(),
		Static:       sub,
		Token:        token,
		Private:      private,
		ExtraOrigins: splitCommaEnv("TRACT_ALLOWED_ORIGINS"),
	}

	// TRACT_ACCESS_LOG=1 prints one line per request: method, path, status, and the
	// caller. Off by default because this is a single-user tool and an always-on access
	// log is noise — but a self-hosted server that cannot say whether a device ever
	// reached it makes "my phone shows no data" unanswerable from the machine side. It
	// deliberately does NOT log the Authorization header, only whether one was present.
	handler := srv.Handler()
	if envTruthy("TRACT_ACCESS_LOG") {
		handler = accessLog(handler)
		log.Printf("access log: on (TRACT_ACCESS_LOG=1)")
	}

	httpSrv := &http.Server{
		Addr: addr,
		// Handler = Routes wrapped in the native-shell CORS layer (see
		// internal/api/cors.go).
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("tract listening on %s (db=%s)", addr, dbPath)
	if err := httpSrv.ListenAndServe(); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// splitCommaEnv parses a comma-separated env var into its non-empty entries;
// unset or empty yields nil.
// envTruthy reads a boolean-ish env var. Accepts the forms a launchd plist or a
// shell export actually produce; anything else (including "0", "false", "off"
// and an empty value) is false, so a typo turns the mode OFF rather than
// silently ON.
func envTruthy(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func splitCommaEnv(key string) []string {
	var out []string
	for _, v := range strings.Split(os.Getenv(key), ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// resolveAddr picks the listen address. TRACT_ADDR wins when set; otherwise a
// bare PORT (the convention PaaS platforms like Fly, Render and Cloud Run inject)
// is honored so the single binary drops into a host with zero config; else
// LOOPBACK — see below.
//
// WHY THE BARE DEFAULT IS 127.0.0.1 AND NOT :8080
// ----------------------------------------------
// It used to be ":8080", which is the wildcard: every interface, including the
// wifi one. The Security section calls the token-less mode "the intended
// zero-config mode for localhost — and only for localhost", and the startup
// warning says "fine on localhost" — but the default address did not implement
// that sentence. Measured on the author's own machine 2026-08-10, 2026-08-16 and
// 2026-08-20: an instance started with no env at all was listening on *:8080 and
// answering anonymous POST /api/items from the LAN, which is a writable library
// plus an SSRF proxy (POST makes the server fetch a caller-supplied URL). The
// documentation was right and the default disagreed with it.
//
// So the bare default now matches the promise. Reaching a token-less instance
// from another device is still possible — it just has to be ASKED for, with
// TRACT_ADDR, which is the difference between a choice and an accident. The two
// deployment paths are untouched: PaaS hosts inject PORT (wildcard, as they must,
// since the proxy connects from outside the container), and any explicit
// TRACT_ADDR wins outright.
func resolveAddr() string {
	if v := os.Getenv("TRACT_ADDR"); v != "" {
		return v
	}
	if p := os.Getenv("PORT"); p != "" {
		return ":" + p
	}
	return "127.0.0.1:8080"
}

// warnIfOpenToTheNetwork prints the loud line when the process is BOTH
// token-less and bound to something other than loopback — the combination that
// makes the library world-writable to whoever shares the network.
//
// The old warning fired on "token is empty" alone and reassured the reader it was
// "fine on localhost" without ever checking whether it was on localhost. A warning
// whose text asserts a precondition it does not test is the same class of defect
// as a check that cannot go red: it was printed, read, and believed, for months,
// by a process listening on the wildcard.
func warnIfOpenToTheNetwork(addr, token string) {
	if token != "" {
		return
	}
	if isLoopback(addr) {
		log.Printf("no TRACT_TOKEN: every route is open, and the listener is loopback-only (%s), so only this machine can reach it", addr)
		return
	}
	log.Printf("WARNING: TRACT_TOKEN is not set AND the listener is %s, which is not loopback — "+
		"every route is open to anything that can reach this address, including POST /api/items, "+
		"which makes this server fetch caller-supplied URLs. Set TRACT_TOKEN (openssl rand -hex 32) "+
		"or bind TRACT_ADDR=127.0.0.1:8080.", addr)
}

// isLoopback reports whether a listen address can only be reached from this
// machine. A bare ":8080" or an empty host is the WILDCARD, not localhost — that
// conflation is the whole bug this function exists to avoid — so anything without
// an explicit loopback host counts as open.
func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// Not host:port at all (e.g. a unix socket path). Unknown is not safe.
		return false
	}
	if host == "" {
		return false // ":8080" — wildcard
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// statusRecorder captures the status code, which http.ResponseWriter does not expose.
// Defaults to 200 because a handler that writes a body without calling WriteHeader has
// implicitly sent one — reporting 0 there would invent a failure.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// accessLog reports who reached this server and what they got. The token itself is
// never logged — only whether the request carried one, which is the bit that explains a
// 401 without putting a credential in a file.
func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)
		auth := "no-token"
		if r.Header.Get("Authorization") != "" {
			auth = "token"
		}
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "-"
		}
		log.Printf("%s %s -> %d (%s, from %s, origin %s, %s)",
			r.Method, r.URL.Path, rec.status, auth, r.RemoteAddr, origin,
			time.Since(start).Round(time.Millisecond))
	})
}
