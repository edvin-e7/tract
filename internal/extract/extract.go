// Package extract fetches a URL and runs readability over it to produce a
// clean title + article body, offline and with no paid APIs.
package extract

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	readability "github.com/go-shiori/go-readability"
)

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

// New returns a Fetcher with a sane default timeout.
func New() *Fetcher {
	return &Fetcher{Client: &http.Client{Timeout: 20 * time.Second}}
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

	art, err := readability.FromReader(resp.Body, u)
	if err != nil {
		return Article{}, fmt.Errorf("readability: %w", err)
	}

	title := strings.TrimSpace(art.Title)
	if title == "" {
		title = u.Host
	}
	return Article{
		URL:      u.String(),
		Title:    title,
		HTML:     art.Content,
		Text:     strings.TrimSpace(art.TextContent),
		Excerpt:  strings.TrimSpace(art.Excerpt),
		SiteName: strings.TrimSpace(art.SiteName),
	}, nil
}
