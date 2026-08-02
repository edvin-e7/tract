package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/edvin-e7/tract/internal/store"
)

// Private mode is the difference between "nobody can write to my instance" and
// "nobody can read my reading list". The token gate already covered writes and
// the server-side fetch — the world-writable and SSRF holes — but every GET was
// open by design, so a public hostname handed the whole library, and every
// highlight in it, to anyone who had the URL.
//
// These drive the REAL routing table, not the middleware in isolation: a gate
// that works but is wired onto the wrong routes is the failure that matters.

func newPrivateServer(t *testing.T, token string, private bool) *http.ServeMux {
	t.Helper()
	path := filepath.Join(t.TempDir(), "private_test.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	s := &Server{
		Store:     st,
		Extractor: fakeExtractor{art: sampleArticle},
		Token:     token,
		Private:   private,
		// A stand-in for the embedded dist, so "GET / still serves the shell"
		// is asserted against real static serving rather than a nil check.
		Static: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<!doctype html><title>tract</title>")}},
	}
	return s.Routes()
}

func status(t *testing.T, mux *http.ServeMux, method, path, token string) int {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Code
}

func TestPrivateMode_ReadsRequireTheToken(t *testing.T) {
	mux := newPrivateServer(t, "s3cret", true)

	for _, path := range []string{"/api/items", "/api/items/1", "/api/search?q=x"} {
		if got := status(t, mux, http.MethodGet, path, ""); got != http.StatusUnauthorized {
			t.Errorf("GET %s without token = %d, want 401 — the reading list is public", path, got)
		}
	}

	// With the token the same routes work: the gate must not be a brick wall.
	if got := status(t, mux, http.MethodGet, "/api/items", "s3cret"); got != http.StatusOK {
		t.Errorf("GET /api/items with token = %d, want 200", got)
	}
	if got := status(t, mux, http.MethodGet, "/api/items", "wrong-token"); got != http.StatusUnauthorized {
		t.Errorf("GET /api/items with a WRONG token = %d, want 401", got)
	}
}

// The PRD's gate names this explicitly: private + no header → 401 on
// GET /api/items while GET / stays 200. The shell has to load before it can ask
// for a token, and serving the app is not serving the data.
func TestPrivateMode_AppShellStaysOpen(t *testing.T) {
	mux := newPrivateServer(t, "s3cret", true)
	if got := status(t, mux, http.MethodGet, "/", ""); got != http.StatusOK {
		t.Errorf("GET / without token = %d, want 200 — the app shell must still load", got)
	}
	if got := status(t, mux, http.MethodGet, "/api/health", ""); got != http.StatusOK {
		t.Errorf("GET /api/health without token = %d, want 200 — health must stay probe-able", got)
	}
}

// Without private mode the LAN workflow is untouched. This is the other half of
// the two-sided check: if it ever fails, the mode stopped being a mode.
func TestNonPrivateMode_ReadsStayOpenEvenWithAToken(t *testing.T) {
	mux := newPrivateServer(t, "s3cret", false)
	if got := status(t, mux, http.MethodGet, "/api/items", ""); got != http.StatusOK {
		t.Errorf("GET /api/items without token in non-private mode = %d, want 200", got)
	}
	// …while writes stay gated regardless of mode.
	if got := status(t, mux, http.MethodDelete, "/api/items/1", ""); got != http.StatusUnauthorized {
		t.Errorf("DELETE without token = %d, want 401 — writes are gated in every mode", got)
	}
}
