package store

import (
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	// On-disk in a temp dir (modernc + WAL + a single conn is happiest with a
	// real file); auto-removed by t.TempDir.
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// TestSearchRoundTrip is the core FTS5 contract: a present term must hit, and an
// absent term must NOT hit. The negative case is the load-bearing half — it
// falsifies "search always returns rows" rather than merely confirming a match.
func TestSearchRoundTrip(t *testing.T) {
	s := newTestStore(t)

	saved, err := s.AddItem(Item{
		URL:   "https://example.com/go-concurrency",
		Title: "A Tour of Go Concurrency",
		Body:  "Goroutines and channels make concurrent programming tractable.",
	})
	if err != nil {
		t.Fatalf("add item: %v", err)
	}

	// Positive: a term in the body matches.
	hits, err := s.Search("goroutines")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit for present term, got %d", len(hits))
	}
	if hits[0].ID != saved.ID {
		t.Fatalf("hit id = %d, want %d", hits[0].ID, saved.ID)
	}

	// Positive: a term in the title matches too.
	if hits, _ := s.Search("concurrency"); len(hits) != 1 {
		t.Fatalf("expected title-term hit, got %d", len(hits))
	}

	// Falsification: a term that appears NOWHERE must return zero rows. If FTS
	// indexing were broken (e.g. triggers missing) this is where it shows.
	absent, err := s.Search("kubernetes")
	if err != nil {
		t.Fatalf("search absent: %v", err)
	}
	if len(absent) != 0 {
		t.Fatalf("expected 0 hits for absent term, got %d", len(absent))
	}
}

// TestDeleteUnindexes proves the FTS delete trigger fires: a deleted item must
// drop out of search results, not linger as a phantom hit.
func TestDeleteUnindexes(t *testing.T) {
	s := newTestStore(t)

	it, err := s.AddItem(Item{URL: "https://x.test/a", Title: "Ephemeral", Body: "transient content here"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if hits, _ := s.Search("transient"); len(hits) != 1 {
		t.Fatalf("pre-delete: expected 1 hit, got %d", len(hits))
	}

	if err := s.DeleteItem(it.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if hits, _ := s.Search("transient"); len(hits) != 0 {
		t.Fatalf("post-delete: expected 0 hits, got %d", len(hits))
	}
}

// TestHighlightsAndNotFound covers the highlight path plus the not-found seams.
func TestHighlightsAndNotFound(t *testing.T) {
	s := newTestStore(t)

	it, err := s.AddItem(Item{URL: "https://x.test/h", Title: "Notes", Body: "body"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	if _, err := s.AddHighlight(it.ID, "a passage worth keeping"); err != nil {
		t.Fatalf("add highlight: %v", err)
	}
	got, err := s.GetItem(it.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Highlights) != 1 || got.Highlights[0].Text != "a passage worth keeping" {
		t.Fatalf("highlights not round-tripped: %+v", got.Highlights)
	}

	// Falsify the not-found contract.
	if _, err := s.GetItem(99999); err != ErrNotFound {
		t.Fatalf("GetItem(missing) = %v, want ErrNotFound", err)
	}
	if err := s.DeleteItem(99999); err != ErrNotFound {
		t.Fatalf("DeleteItem(missing) = %v, want ErrNotFound", err)
	}
	if _, err := s.AddHighlight(99999, "x"); err != ErrNotFound {
		t.Fatalf("AddHighlight(missing) = %v, want ErrNotFound", err)
	}
}
