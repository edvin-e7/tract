package api

import (
	"io/fs"
	"net/http"
	"strings"
)

// spaHandler serves the built frontend from dist, falling back to index.html for
// client-side routes (anything that isn't an existing file and isn't /api/...).
func spaHandler(dist fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(dist, p); err != nil {
			// Unknown path → let the SPA router handle it.
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
