// CORS for the native shells. The Capacitor iOS/Android apps bundle the SPA
// and serve it from a WebView-local origin (capacitor://localhost on iOS,
// https://localhost on Android), so their API calls are cross-origin by
// construction. Those two origins are allowed by default; they are WebView
// schemes a real website can never run under, and the browser (not the caller)
// sets the Origin header, so this does not widen what a hostile web page can
// do — cookies and ambient auth don't exist here, and the token still has to
// be presented explicitly.
//
// ExtraOrigins (comma-separated in TRACT_ALLOWED_ORIGINS, parsed in main)
// opts in additional origins, e.g. a Vite dev/preview server during testing.
package api

import (
	"net/http"
	"strings"
)

var nativeOrigins = map[string]bool{
	"capacitor://localhost": true, // Capacitor iOS WebView
	"https://localhost":     true, // Capacitor Android WebView
}

// Handler wraps Routes with the CORS layer. main.go serves this; tests that
// don't care about CORS keep hitting Routes() directly.
func (s *Server) Handler() http.Handler {
	allowed := make(map[string]bool, len(nativeOrigins)+len(s.ExtraOrigins))
	for o := range nativeOrigins {
		allowed[o] = true
	}
	for _, o := range s.ExtraOrigins {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			allowed[o] = true
		}
	}

	mux := s.Routes()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			// Any response may now vary on Origin — mark it for caches even
			// when the origin is refused, so an ACAO-less response can't be
			// cached and replayed for an allowed one (or vice versa).
			w.Header().Add("Vary", "Origin")
		}
		if origin != "" && allowed[origin] {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			if r.Method == http.MethodOptions {
				h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				h.Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		} else if origin != "" && r.Method == http.MethodOptions {
			// Preflight from a disallowed origin: answer here — the mux has no
			// OPTIONS routes and would 405 with a misleading Allow header.
			w.WriteHeader(http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
