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

	// permissiveFetcher, not New(): the test server lives on loopback, which
	// the production SSRF guard refuses by design (see netguard_test.go).
	art, err := permissiveFetcher().Fetch(context.Background(), srv.URL)
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

// TestHTMLPolicyContract pins the sanitizer's allow/deny contract directly, so a
// future policy swap can't silently loosen it. It documents exactly what a reader
// needs to keep (formatting, links, images, code) versus what must never survive
// (scripts, iframes, inline event handlers, javascript: URLs). Testing the policy
// value itself (not a network round-trip) keeps it deterministic and offline.
func TestHTMLPolicyContract(t *testing.T) {
	const in = `<p>text <strong>bold</strong> <em>em</em></p>` +
		`<a href="https://example.com/page">safe link</a>` +
		`<a href="javascript:alert(1)">js link</a>` +
		`<img src="https://example.com/a.png" alt="pic" onerror="pwn()">` +
		`<pre><code>fmt.Println("hi")</code></pre>` +
		`<h2 onclick="pwn()">Heading</h2>` +
		`<blockquote>quoted</blockquote>` +
		`<script>window.__pwned=1</script>` +
		`<iframe src="https://evil.example"></iframe>` +
		`<object data="x"></object>`

	got := strings.ToLower(htmlPolicy.Sanitize(in))

	// Must keep — the formatting a reader legitimately needs.
	for _, want := range []string{"<strong>", "<em>", "<a href=", "https://example.com/page", "<img", "<pre>", "<code>", "<blockquote>", "<h2>", "heading"} {
		if !strings.Contains(got, want) {
			t.Errorf("sanitizer dropped safe content %q; got: %s", want, got)
		}
	}
	// Must strip — anything executable or embeddable.
	for _, bad := range []string{"<script", "<iframe", "<object", "onerror", "onclick", "javascript:"} {
		if strings.Contains(got, bad) {
			t.Errorf("sanitizer left dangerous content %q; got: %s", bad, got)
		}
	}
}
