package extract

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A page whose article body carries an executable payload. Readability keeps the
// prose; sanitization must drop the <script> and the onerror handler before the
// HTML is ever stored. The negative assertions are load-bearing — a no-op
// sanitizer would leave them in and fail.
const maliciousPage = `<!doctype html><html><head><title>Trusted Title</title></head>
<body><article>
<h1>Trusted Title</h1>
<p>The quick brown fox jumps over the lazy dog, and again, and again, so readability keeps this paragraph.</p>
<p>More substantial prose so the extractor treats this as the real article body rather than chrome.</p>
<script>window.__pwned = true;</script>
<img src="x" onerror="window.__pwned = true">
<a href="javascript:alert(1)">click</a>
</article></body></html>`

func TestSanitizeStripsExecutableContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(maliciousPage))
	}))
	defer srv.Close()

	art, err := New().Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	// Positive: the real prose survived (proves we didn't just nuke everything).
	if !strings.Contains(art.HTML, "quick brown fox") {
		t.Fatalf("expected article prose to survive sanitization, got: %q", art.HTML)
	}
	// Negative (falsifying): nothing executable may remain.
	for _, bad := range []string{"<script", "onerror", "javascript:"} {
		if strings.Contains(strings.ToLower(art.HTML), bad) {
			t.Fatalf("sanitizer left executable content %q in: %q", bad, art.HTML)
		}
	}
}
