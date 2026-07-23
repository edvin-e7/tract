// Command tract is the single-binary read-later server: it serves the JSON API
// and the embedded frontend build from one process.
package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
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

	token := os.Getenv("TRACT_TOKEN")
	if token == "" {
		log.Printf("WARNING: TRACT_TOKEN is not set — every route is open (fine on localhost; NEVER expose this instance publicly without a token)")
	}

	srv := &api.Server{
		Store:        st,
		Extractor:    extract.New(),
		Static:       sub,
		Token:        token,
		ExtraOrigins: splitCommaEnv("TRACT_ALLOWED_ORIGINS"),
	}

	httpSrv := &http.Server{
		Addr: addr,
		// Handler = Routes wrapped in the native-shell CORS layer (see
		// internal/api/cors.go).
		Handler:           srv.Handler(),
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

// splitCommaEnv parses a comma-separated env var into its non-empty entries;
// unset or empty yields nil.
func splitCommaEnv(key string) []string {
	var out []string
	for _, v := range strings.Split(os.Getenv(key), ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
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
