package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// newCORSHandler builds the full CORS-wrapped handler around a store-less
// Server — /api/health is the probe route, so no store or extractor is needed.
func newCORSHandler(extra ...string) http.Handler {
	s := &Server{ExtraOrigins: extra}
	return s.Handler()
}

func doCORS(t *testing.T, h http.Handler, method, origin string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/api/health", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCORSPreflightFromNativeShell(t *testing.T) {
	rec := doCORS(t, newCORSHandler(), http.MethodOptions, "capacitor://localhost")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "capacitor://localhost" {
		t.Fatalf("ACAO = %q, want capacitor://localhost", got)
	}
	// The token header must be preflight-approved or every mutating call from
	// the shell dies in the browser before reaching the server.
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Authorization, Content-Type" {
		t.Fatalf("allow-headers = %q", got)
	}
}

func TestCORSPreflightFromUnknownOriginRefused(t *testing.T) {
	rec := doCORS(t, newCORSHandler(), http.MethodOptions, "https://evil.example")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("preflight status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("ACAO leaked to unknown origin: %q", got)
	}
}

func TestCORSSimpleRequestEchoesAllowedOrigin(t *testing.T) {
	rec := doCORS(t, newCORSHandler(), http.MethodGet, "https://localhost")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://localhost" {
		t.Fatalf("ACAO = %q, want https://localhost", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("Vary = %q, want Origin", got)
	}
}

func TestCORSUnknownOriginGetsNoACAOButStillServes(t *testing.T) {
	// A same-API GET from an unlisted origin still runs (the API is public-read
	// by design); the browser just gets no CORS grant to expose it.
	rec := doCORS(t, newCORSHandler(), http.MethodGet, "https://evil.example")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("ACAO leaked to unknown origin: %q", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("Vary = %q, want Origin (refusals must be cache-partitioned too)", got)
	}
}

func TestCORSExtraOriginsHonoredAndNormalized(t *testing.T) {
	// Trailing slash and padding come in from env-var config; both must match
	// the browser's slash-less Origin header.
	rec := doCORS(t, newCORSHandler(" http://localhost:4173/ "), http.MethodGet, "http://localhost:4173")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:4173" {
		t.Fatalf("ACAO = %q, want http://localhost:4173", got)
	}
}

func TestCORSNoOriginHeaderUntouched(t *testing.T) {
	rec := doCORS(t, newCORSHandler(), http.MethodGet, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("ACAO on origin-less request: %q", got)
	}
	if got := rec.Header().Get("Vary"); got != "" {
		t.Fatalf("Vary on origin-less request: %q", got)
	}
}
