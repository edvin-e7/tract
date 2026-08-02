// Package api wires HTTP handlers onto a stdlib net/http ServeMux using Go 1.22
// method+path patterns (e.g. "POST /api/items"). No router dependency.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/edvin-e7/tract/internal/extract"
	"github.com/edvin-e7/tract/internal/store"
)

// Extractor is the slice of *extract.Fetcher the API needs; an interface keeps
// handlers testable without real network fetches.
type Extractor interface {
	Fetch(ctx context.Context, url string) (extract.Article, error)
}

// Server holds dependencies for the HTTP handlers.
type Server struct {
	Store     *store.Store
	Extractor Extractor
	// Static is the embedded frontend dist; nil disables static serving.
	Static fs.FS
	// Token, when non-empty, is required (as `Authorization: Bearer <Token>`)
	// on every mutating route and on the URL-fetching route. Empty keeps all
	// routes open — local single-user mode. See auth.go.
	Token string
	// Private, when true, extends the token requirement to the READ routes as
	// well. Writes and the server-side fetch are gated either way — those are
	// the world-writable/SSRF holes. Reads are a different question: with the
	// gate as it stood, anyone who had the URL could page through the whole
	// reading list and every highlight in it. That is fine on a LAN and wrong
	// on a public hostname, so it is a mode rather than a default.
	//
	// Private has no effect without a Token: a gate with nothing to check would
	// lock the owner out of their own instance and protect nobody. main.go
	// refuses that combination loudly rather than starting half-secured.
	Private bool
	// ExtraOrigins are CORS origins allowed on top of the built-in native-shell
	// origins (TRACT_ALLOWED_ORIGINS, comma-separated). See cors.go.
	ExtraOrigins []string
}

// readGate wraps a read-only handler: gated in private mode, open otherwise.
func (s *Server) readGate(next http.HandlerFunc) http.HandlerFunc {
	if !s.Private {
		return next
	}
	return s.requireToken(next)
}

// Routes builds the ServeMux. API routes first; a catch-all serves the SPA.
// Mutating routes (and the server-side fetch) sit behind requireToken; the read
// routes sit behind readGate, which is that same check only in private mode.
//
// GET / stays open in every mode on purpose: the SPA shell has to load before
// it can ask for a token, and serving the app is not serving the data.
func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/items", s.requireToken(s.addItem))
	mux.HandleFunc("GET /api/items", s.readGate(s.listItems))
	mux.HandleFunc("GET /api/items/{id}", s.readGate(s.getItem))
	mux.HandleFunc("DELETE /api/items/{id}", s.requireToken(s.deleteItem))
	mux.HandleFunc("POST /api/items/{id}/highlights", s.requireToken(s.addHighlight))
	mux.HandleFunc("DELETE /api/items/{id}/highlights/{hid}", s.requireToken(s.deleteHighlight))
	mux.HandleFunc("GET /api/search", s.readGate(s.search))
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	if s.Static != nil {
		mux.Handle("GET /", spaHandler(s.Static))
	}
	return mux
}

func (s *Server) addItem(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if strings.TrimSpace(body.URL) == "" {
		writeErr(w, http.StatusBadRequest, "url is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	art, err := s.Extractor.Fetch(ctx, body.URL)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "could not fetch article: "+err.Error())
		return
	}

	it, err := s.Store.AddItem(store.Item{
		URL:      art.URL,
		Title:    art.Title,
		Body:     art.Text,
		HTML:     art.HTML,
		Excerpt:  art.Excerpt,
		SiteName: art.SiteName,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not save item")
		return
	}
	writeJSON(w, http.StatusCreated, it)
}

func (s *Server) listItems(w http.ResponseWriter, _ *http.Request) {
	items, err := s.Store.ListItems()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not list items")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) getItem(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	it, err := s.Store.GetItem(id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load item")
		return
	}
	writeJSON(w, http.StatusOK, it)
}

func (s *Server) deleteItem(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	err := s.Store.DeleteItem(id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not delete item")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) addHighlight(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if strings.TrimSpace(body.Text) == "" {
		writeErr(w, http.StatusBadRequest, "text is required")
		return
	}
	h, err := s.Store.AddHighlight(id, body.Text)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not save highlight")
		return
	}
	writeJSON(w, http.StatusCreated, h)
}

func (s *Server) deleteHighlight(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	hid, err := strconv.ParseInt(r.PathValue("hid"), 10, 64)
	if err != nil || hid <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid highlight id")
		return
	}
	err = s.Store.DeleteHighlight(id, hid)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "highlight not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not delete highlight")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	items, err := s.Store.Search(q)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "search failed")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// pathID parses the {id} path value, writing a 400 and returning false on error.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
