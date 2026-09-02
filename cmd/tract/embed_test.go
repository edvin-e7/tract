package main

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// The embedded frontend must be SELF-CONSISTENT: every asset index.html asks for
// has to exist inside the same embedded tree.
//
// WHY THIS GATE EXISTS. `//go:embed all:dist` embeds whatever is on disk at compile
// time and never complains about what is missing, and cmd/tract/dist/assets/ is
// gitignored while dist/index.html is tracked. So a fresh clone compiles cleanly into
// a binary whose index.html requests two hashed bundles that are not in the binary at
// all. That alone would be a 404 — survivable, and visible in a browser console.
//
// spaHandler makes it worse than a 404, which is the actual reason this is a test and
// not a comment: any path that does not exist is rewritten to "/" and served
// index.html. So <script type="module" src="./assets/index-*.js"> comes back as
// text/html with 200 OK, the browser refuses to execute it, and the page is blank
// with no error anywhere. Nothing in `go vet`, `go test` or CI's separate frontend job
// can see it — the Go job never runs the frontend build.
//
// The committed file is meant to be a placeholder ("makes the embed valid before the
// first npm run build"), but scripts/build.sh does `rm -rf` + copy, so a real build
// overwrites it and the artifact gets committed by accident. A placeholder that
// references nothing passes this test; a stale build does not.
func TestEmbeddedIndexReferencesOnlyEmbeddedAssets(t *testing.T) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}
	raw, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		t.Fatalf("dist/index.html must exist — go:embed keeps it valid: %v", err)
	}
	// Strip HTML comments FIRST. A comment that names an asset path is documentation,
	// not a request the browser makes — and the placeholder's own comment explains this
	// very failure, so a byte-level match would fail the gate on its own explanation:
	// a check has to read the surface a browser sees, not the file's bytes.
	body := regexp.MustCompile(`(?s)<!--.*?-->`).ReplaceAllString(string(raw), "")
	// src="..." / href="..." pointing at a local path (not http(s):// or data:).
	re := regexp.MustCompile(`(?:src|href)="(\./[^"]+|/[^/][^"]*)"`)
	var missing []string
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		ref := strings.TrimPrefix(strings.TrimPrefix(m[1], "./"), "/")
		if ref == "" {
			continue
		}
		if _, err := fs.Stat(sub, ref); err != nil {
			missing = append(missing, ref)
		}
	}
	if len(missing) > 0 {
		var have []string
		_ = fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				have = append(have, p)
			}
			return nil
		})
		t.Fatalf("dist/index.html references %d file(s) that are not embedded: %v\n"+
			"embedded tree is: %v\n"+
			"A stale build artifact was committed over the placeholder. spaHandler rewrites\n"+
			"every missing path to \"/\", so these come back as index.html with 200 OK and the\n"+
			"page renders blank with no error. Either run scripts/build.sh (real assets land\n"+
			"next to index.html) or restore the reference-free placeholder before committing.",
			len(missing), missing, have)
	}
}
