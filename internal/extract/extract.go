// Package extract fetches a URL and runs readability over it to produce a
// clean title + article body, offline and with no paid APIs.
package extract

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	readability "github.com/go-shiori/go-readability"
	"github.com/microcosm-cc/bluemonday"
)

// maxFetchBytes caps how much of a fetched page is read. The README's Fly
// deploy runs on a 256 MB VM and readability buffers the whole document, so an
// unbounded (or hostile) response is an out-of-memory lever. 10 MiB is far
// beyond any real article page.
const maxFetchBytes = 10 << 20

// htmlPolicy sanitizes readability's output before it is ever stored or served.
// Article HTML is untrusted (it came from an arbitrary page), so without this a
// saved <script> or onerror= is a stored-XSS waiting to fire in the reader view.
// UGCPolicy allows the formatting a reader needs (headings, links, images, code)
// and strips everything executable. Compiled once; safe for concurrent use.
var htmlPolicy = bluemonday.UGCPolicy()

// Article is the distilled result of fetching and cleaning a page.
type Article struct {
	URL      string
	Title    string
	HTML     string // cleaned article markup
	Text     string // plain text (used for FTS + reader fallback)
	Excerpt  string
	SiteName string
}

// Fetcher pulls and extracts an article. The http.Client is injectable so tests
// can serve fixtures without network access.
type Fetcher struct {
	Client *http.Client
}

// New returns a Fetcher with sane timeouts and the SSRF dial guard (see
// netguard.go): connections to private/loopback/link-local/metadata addresses
// are refused at dial time, including on redirect hops.
func New() *Fetcher {
	return &Fetcher{Client: newHTTPClient(addrAllowed)}
}

// Fetch retrieves rawURL and extracts its readable content. Network and parse
// failures are wrapped; the caller decides how to surface them.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (Article, error) {
	u, err := url.ParseRequestURI(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return Article{}, fmt.Errorf("invalid url: %q", rawURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Article{}, err
	}
	// A real UA avoids the trivial bot-block that returns an empty body.
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; TractReader/0.1; +https://github.com/edvin-e7/tract)")

	resp, err := f.Client.Do(req)
	if err != nil {
		return Article{}, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Article{}, fmt.Errorf("fetch: status %d", resp.StatusCode)
	}
	if resp.ContentLength > maxFetchBytes {
		return Article{}, fmt.Errorf("fetch: response is %d bytes, over the %d MiB limit", resp.ContentLength, maxFetchBytes>>20)
	}

	// Read at most one byte over the cap: if the parser drained the reader,
	// the body exceeded the limit and the article is rejected rather than
	// silently stored truncated.
	body := &io.LimitedReader{R: resp.Body, N: maxFetchBytes + 1}
	art, err := readability.FromReader(body, u)
	if err != nil {
		return Article{}, fmt.Errorf("readability: %w", err)
	}
	if body.N <= 0 {
		return Article{}, fmt.Errorf("fetch: response exceeds the %d MiB limit", maxFetchBytes>>20)
	}

	title := strings.TrimSpace(art.Title)
	if title == "" {
		title = u.Host
	}
	return Article{
		URL:      u.String(),
		Title:    title,
		HTML:     htmlPolicy.Sanitize(art.Content),
		Text:     strings.TrimSpace(art.TextContent),
		Excerpt:  strings.TrimSpace(art.Excerpt),
		SiteName: strings.TrimSpace(art.SiteName),
	}, nil
}
