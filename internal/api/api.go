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
}

// Routes builds the ServeMux. API routes first; a catch-all serves the SPA.
func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/items", s.addItem)
	mux.HandleFunc("GET /api/items", s.listItems)
	mux.HandleFunc("GET /api/items/{id}", s.getItem)
	mux.HandleFunc("DELETE /api/items/{id}", s.deleteItem)
	mux.HandleFunc("POST /api/items/{id}/highlights", s.addHighlight)
	mux.HandleFunc("GET /api/search", s.search)
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
