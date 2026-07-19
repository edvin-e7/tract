package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/edvin-e7/tract/internal/store"
)

// newTokenServer mirrors newTestServer but configures a bearer token, so these
// tests drive the REAL routing table with the auth wrapper exactly as
// production wires it — not the middleware func in isolation.
func newTokenServer(t *testing.T, token string) *http.ServeMux {
	t.Helper()
	path := filepath.Join(t.TempDir(), "auth_test.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	s := &Server{Store: st, Extractor: fakeExtractor{art: sampleArticle}, Token: token}
	return s.Routes()
}

// doTok issues a request with an optional Authorization header value.
func doTok(t *testing.T, mux *http.ServeMux, method, target, authHeader string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("marshal body: %v", err)
		}
	}
	req := httptest.NewRequest(method, target, &buf)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

const testToken = "s3cret-test-token"

// TestMutatingRoutesRequireToken sweeps every mutating route through the real
// mux: no header and a wrong token must both 401 (with WWW-Authenticate), and
// the handler must not have run.
func TestMutatingRoutesRequireToken(t *testing.T) {
	mux := newTokenServer(t, testToken)

	routes := []struct {
		method, target string
		body           any
	}{
		{"POST", "/api/items", map[string]string{"url": "https://example.com/x"}},
		{"DELETE", "/api/items/1", nil},
		{"POST", "/api/items/1/highlights", map[string]string{"text": "hi"}},
		{"DELETE", "/api/items/1/highlights/1", nil},
	}
	for _, rt := range routes {
		for name, header := range map[string]string{
			"no header":    "",
			"wrong token":  "Bearer definitely-not-it",
			"wrong scheme": "Basic " + testToken,
		} {
			rec := doTok(t, mux, rt.method, rt.target, header, rt.body)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s (%s): code = %d, want 401", rt.method, rt.target, name, rec.Code)
			}
			if rec.Header().Get("WWW-Authenticate") == "" {
				t.Fatalf("%s %s (%s): missing WWW-Authenticate header", rt.method, rt.target, name)
			}
		}
	}

	// Nothing may have been written through any of those attempts.
	if rec := doTok(t, mux, "GET", "/api/items", "", nil); rec.Body.String() == "" || rec.Code != http.StatusOK {
		t.Fatalf("list after 401s: code = %d", rec.Code)
	} else {
		var items []store.Item
		if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil || len(items) != 0 {
			t.Fatalf("expected empty library after rejected writes, got %q", rec.Body.String())
		}
	}
}

// TestCorrectTokenPassesThrough proves the gate is a pass-through, not a wall:
// with the right bearer the full add → highlight → delete flow works against
// the real handlers.
func TestCorrectTokenPassesThrough(t *testing.T) {
	mux := newTokenServer(t, testToken)
	auth := "Bearer " + testToken

	rec := doTok(t, mux, "POST", "/api/items", auth, map[string]string{"url": "https://example.com/x"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("add with token: code = %d, want 201 (body %q)", rec.Code, rec.Body.String())
	}
	created := decode[store.Item](t, rec)

	base := "/api/items/" + itoa(created.ID)
	if rec := doTok(t, mux, "POST", base+"/highlights", auth, map[string]string{"text": "keep this"}); rec.Code != http.StatusCreated {
		t.Fatalf("highlight with token: code = %d, want 201", rec.Code)
	}
	if rec := doTok(t, mux, "DELETE", base, auth, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete with token: code = %d, want 204", rec.Code)
	}
}

// TestReadRoutesStayOpenWithToken pins the deliberate design choice: read-only
// GETs (list, get, search, health) do not require the token even when one is
// configured.
func TestReadRoutesStayOpenWithToken(t *testing.T) {
	mux := newTokenServer(t, testToken)
	created := decode[store.Item](t, doTok(t, mux, "POST", "/api/items", "Bearer "+testToken, map[string]string{"url": "https://example.com/x"}))

	for _, target := range []string{
		"/api/items",
		"/api/items/" + itoa(created.ID),
		"/api/search?q=ServeMux",
		"/api/health",
	} {
		if rec := doTok(t, mux, "GET", target, "", nil); rec.Code != http.StatusOK {
			t.Fatalf("GET %s without token: code = %d, want 200 (reads are open by design)", target, rec.Code)
		}
	}
}

// TestEmptyTokenKeepsEverythingOpen falsifies "the gate is always on": with no
// token configured (local mode) a bare mutating request must succeed exactly
// as before the auth feature existed.
func TestEmptyTokenKeepsEverythingOpen(t *testing.T) {
	mux := newTokenServer(t, "")
	rec := doTok(t, mux, "POST", "/api/items", "", map[string]string{"url": "https://example.com/x"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("add without token in open mode: code = %d, want 201 (body %q)", rec.Code, rec.Body.String())
	}
}
