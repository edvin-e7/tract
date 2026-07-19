package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// requireToken gates a handler behind `Authorization: Bearer <TRACT_TOKEN>`.
// It wraps every mutating route plus the server-side URL fetch (POST
// /api/items) — the two things that make an unprotected public deploy
// world-writable and an SSRF proxy. Read-only GETs stay open by design.
//
// When no token is configured (Token == "") the gate is a no-op, preserving
// the zero-config local workflow; main.go logs a loud warning for that mode.
func (s *Server) requireToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.Token == "" {
			next(w, r)
			return
		}
		const prefix = "Bearer "
		auth := r.Header.Get("Authorization")
		if len(auth) > len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
			got := strings.TrimSpace(auth[len(prefix):])
			// Constant-time compare so the token can't be guessed
			// byte-by-byte from response timing.
			if subtle.ConstantTimeCompare([]byte(got), []byte(s.Token)) == 1 {
				next(w, r)
				return
			}
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="tract"`)
		writeErr(w, http.StatusUnauthorized, "missing or invalid bearer token")
	}
}
