// Command tract is the single-binary read-later server: it serves the JSON API
// and the embedded frontend build from one process.
package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
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

	srv := &api.Server{
		Store:     st,
		Extractor: extract.New(),
		Static:    sub,
	}

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Routes(),
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

// resolveAddr picks the listen address. TRACT_ADDR wins when set; otherwise a
// bare PORT (the convention PaaS platforms like Fly, Render and Cloud Run inject)
// is honored so the single binary drops into a host with zero config; else :8080.
func resolveAddr() string {
	if v := os.Getenv("TRACT_ADDR"); v != "" {
		return v
	}
	if p := os.Getenv("PORT"); p != "" {
		return ":" + p
	}
	return ":8080"
}
