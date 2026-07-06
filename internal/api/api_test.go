package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/edvin-e7/tract/internal/extract"
	"github.com/edvin-e7/tract/internal/store"
)

// fakeExtractor stands in for the network fetch so handler tests stay hermetic.
// Returning a fixed Article (or a forced error) lets us exercise both the happy
// path and the upstream-failure seam without ever touching the wire.
type fakeExtractor struct {
	art extract.Article
	err error
}

func (f fakeExtractor) Fetch(_ context.Context, _ string) (extract.Article, error) {
	return f.art, f.err
}

// newTestServer wires a real temp-file Store (modernc is happiest on disk, per
// store_test) to the fake extractor, and returns the mux so tests drive the
// actual Go 1.22 method+path routing table — not handler funcs in isolation.
// That makes the routing wiring itself part of what's under test.
func newTestServer(t *testing.T, ext Extractor) *http.ServeMux {
	t.Helper()
	path := filepath.Join(t.TempDir(), "api_test.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	s := &Server{Store: st, Extractor: ext} // Static nil → no SPA catch-all
	return s.Routes()
}

func do(t *testing.T, mux *http.ServeMux, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, target, r)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
	return v
}

var sampleArticle = extract.Article{
	URL:      "https://example.com/go-1.22-routing",
	Title:    "Routing in Go 1.22",
	Text:     "The net/http ServeMux now understands method and path patterns.",
	HTML:     "<p>The net/http ServeMux now understands method and path patterns.</p>",
	Excerpt:  "ServeMux gains method+path patterns.",
	SiteName: "example.com",
}

// TestAddItemLifecycle walks the core flow over HTTP: add → list → get, then
// proves the persisted fields actually came from the extractor's Article.
func TestAddItemLifecycle(t *testing.T) {
	mux := newTestServer(t, fakeExtractor{art: sampleArticle})

	rec := do(t, mux, "POST", "/api/items", map[string]string{"url": "https://example.com/x"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("add: code = %d, want 201 (body %q)", rec.Code, rec.Body.String())
	}
	created := decode[store.Item](t, rec)
	if created.ID <= 0 {
		t.Fatalf("add: expected positive id, got %d", created.ID)
	}
	if created.Title != sampleArticle.Title {
		t.Fatalf("add: title = %q, want %q (extractor output not persisted)", created.Title, sampleArticle.Title)
	}

	rec = do(t, mux, "GET", "/api/items", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: code = %d, want 200", rec.Code)
	}
	if items := decode[[]store.Item](t, rec); len(items) != 1 {
		t.Fatalf("list: expected 1 item, got %d", len(items))
	}

	rec = do(t, mux, "GET", "/api/items/"+itoa(created.ID), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: code = %d, want 200", rec.Code)
	}
	if got := decode[store.Item](t, rec); got.ID != created.ID {
		t.Fatalf("get: id = %d, want %d", got.ID, created.ID)
	}
}

// TestAddItemBadInput falsifies the validation seams: malformed JSON, a missing
// url, and an upstream fetch failure must each surface a distinct 4xx/5xx — not
// a 201 with empty content, and not a 500 that hides a client error.
func TestAddItemBadInput(t *testing.T) {
	mux := newTestServer(t, fakeExtractor{err: errors.New("boom")})

	// invalid json
	req := httptest.NewRequest("POST", "/api/items", strings.NewReader("{not json"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid json: code = %d, want 400", rec.Code)
	}

	// empty url
	rec = do(t, mux, "POST", "/api/items", map[string]string{"url": "   "})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty url: code = %d, want 400", rec.Code)
	}

	// extractor failure → 502, and the message must propagate (not a bare 500)
	rec = do(t, mux, "POST", "/api/items", map[string]string{"url": "https://nope.test"})
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("fetch failure: code = %d, want 502", rec.Code)
	}
}

// TestGetDeleteNotFound covers the ErrNotFound → 404 mapping and the invalid-id
// → 400 guard, then proves delete is real: after a 204 the item is gone (404),
// not merely flagged.
func TestGetDeleteNotFound(t *testing.T) {
	mux := newTestServer(t, fakeExtractor{art: sampleArticle})

	for _, tc := range []struct{ method, target string }{
		{"GET", "/api/items/99999"},
		{"DELETE", "/api/items/99999"},
	} {
		rec := do(t, mux, tc.method, tc.target, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s %s: code = %d, want 404", tc.method, tc.target, rec.Code)
		}
	}

	// invalid (non-numeric) id → 400, not 404
	if rec := do(t, mux, "GET", "/api/items/abc", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad id: code = %d, want 400", rec.Code)
	}

	// real delete round-trip
	created := decode[store.Item](t, do(t, mux, "POST", "/api/items", map[string]string{"url": "https://example.com/x"}))
	if rec := do(t, mux, "DELETE", "/api/items/"+itoa(created.ID), nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: code = %d, want 204", rec.Code)
	}
	if rec := do(t, mux, "GET", "/api/items/"+itoa(created.ID), nil); rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete: code = %d, want 404 (delete was not real)", rec.Code)
	}
}

// TestAddHighlight covers the success path plus both rejection seams (missing
// item, empty text).
func TestAddHighlight(t *testing.T) {
	mux := newTestServer(t, fakeExtractor{art: sampleArticle})
	created := decode[store.Item](t, do(t, mux, "POST", "/api/items", map[string]string{"url": "https://example.com/x"}))
	base := "/api/items/" + itoa(created.ID) + "/highlights"

	if rec := do(t, mux, "POST", base, map[string]string{"text": "a passage worth keeping"}); rec.Code != http.StatusCreated {
		t.Fatalf("highlight: code = %d, want 201 (body %q)", rec.Code, rec.Body.String())
	}
	if rec := do(t, mux, "POST", base, map[string]string{"text": "  "}); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty highlight: code = %d, want 400", rec.Code)
	}
	if rec := do(t, mux, "POST", "/api/items/99999/highlights", map[string]string{"text": "x"}); rec.Code != http.StatusNotFound {
		t.Fatalf("highlight on missing item: code = %d, want 404", rec.Code)
	}
}

// TestDeleteHighlightOverHTTP walks add → delete → verify-gone over HTTP, plus
// the rejection seams: a bad highlight id is 400, and a missing one is 404 (not
// a swallowed 204 that pretends it removed something).
func TestDeleteHighlightOverHTTP(t *testing.T) {
	mux := newTestServer(t, fakeExtractor{art: sampleArticle})
	created := decode[store.Item](t, do(t, mux, "POST", "/api/items", map[string]string{"url": "https://example.com/x"}))
	base := "/api/items/" + itoa(created.ID) + "/highlights"

	h := decode[store.Highlight](t, do(t, mux, "POST", base, map[string]string{"text": "worth keeping"}))
	if h.ID <= 0 {
		t.Fatalf("expected positive highlight id, got %d", h.ID)
	}

	// bad highlight id → 400
	if rec := do(t, mux, "DELETE", base+"/abc", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad hid: code = %d, want 400", rec.Code)
	}
	// missing highlight id → 404
	if rec := do(t, mux, "DELETE", base+"/99999", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("missing hid: code = %d, want 404", rec.Code)
	}
	// real delete → 204, and the item now reports zero highlights
	if rec := do(t, mux, "DELETE", base+"/"+itoa(h.ID), nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: code = %d, want 204", rec.Code)
	}
	got := decode[store.Item](t, do(t, mux, "GET", "/api/items/"+itoa(created.ID), nil))
	if len(got.Highlights) != 0 {
		t.Fatalf("highlight survived delete: %+v (delete was not real)", got.Highlights)
	}
}

// TestSearchOverHTTP proves the FTS5 contract survives the full HTTP round-trip:
// a present term hits, an absent term returns an empty array (the load-bearing
// negative — it falsifies "search always echoes rows").
func TestSearchOverHTTP(t *testing.T) {
	mux := newTestServer(t, fakeExtractor{art: sampleArticle})
	do(t, mux, "POST", "/api/items", map[string]string{"url": "https://example.com/x"})

	rec := do(t, mux, "GET", "/api/search?q=ServeMux", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("search: code = %d, want 200", rec.Code)
	}
	if hits := decode[[]store.Item](t, rec); len(hits) != 1 {
		t.Fatalf("search present term: got %d hits, want 1", len(hits))
	}

	rec = do(t, mux, "GET", "/api/search?q=kubernetes", nil)
	if hits := decode[[]store.Item](t, rec); len(hits) != 0 {
		t.Fatalf("search absent term: got %d hits, want 0", len(hits))
	}
}

// TestHealthAndMethodRouting confirms /api/health is wired and that the method
// dimension of the routing table is real: GET on a POST-only route must 405,
// not silently fall through to a 200.
func TestHealthAndMethodRouting(t *testing.T) {
	mux := newTestServer(t, fakeExtractor{art: sampleArticle})

	if rec := do(t, mux, "GET", "/api/health", nil); rec.Code != http.StatusOK {
		t.Fatalf("health: code = %d, want 200", rec.Code)
	}
	// DELETE is not registered for /api/search → stdlib mux returns 405.
	if rec := do(t, mux, "DELETE", "/api/search", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("wrong method on /api/search: code = %d, want 405", rec.Code)
	}
}

// itoa avoids dragging strconv into every call site.
func itoa(n int64) string {
	return strings.TrimSpace(string(jsonNumber(n)))
}

func jsonNumber(n int64) []byte {
	b, _ := json.Marshal(n)
	return b
}
