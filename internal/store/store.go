// Package store is the persistence layer: SQLite (pure-Go, no CGO) with an
// FTS5 virtual table mirroring item text for full-text search.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	// Pure-Go SQLite. Driver name is "sqlite" (NOT "sqlite3"); FTS5 is compiled
	// in, but loadable extensions are unavailable — so search uses the built-in
	// FTS5 module, never an external one.
	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a lookup by id matches no row.
var ErrNotFound = errors.New("store: not found")

// Item is a saved article. Body holds the extracted plain text used for both
// the reader view and FTS indexing; HTML holds the cleaned article markup.
type Item struct {
	ID         int64       `json:"id"`
	URL        string      `json:"url"`
	Title      string      `json:"title"`
	Body       string      `json:"body"`
	HTML       string      `json:"html"`
	Excerpt    string      `json:"excerpt"`
	SiteName   string      `json:"siteName"`
	CreatedAt  time.Time   `json:"createdAt"`
	Highlights []Highlight `json:"highlights,omitempty"`
	// HighlightCount is populated by list/search queries (where the full
	// Highlights slice is not loaded) so the library can show a per-row count.
	HighlightCount int `json:"highlightCount"`
}

// Highlight is a user-saved passage attached to an item.
type Highlight struct {
	ID        int64     `json:"id"`
	ItemID    int64     `json:"itemId"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}

// Store wraps the database handle.
type Store struct {
	db *sql.DB
}

// Open opens (and migrates) the database at path. WAL + a busy timeout are set
// via DSN pragmas so concurrent reads during a write don't fail with SQLITE_BUSY.
func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	// modernc's pure-Go engine isn't goroutine-safe per *conn; a single conn
	// serializes writes and sidesteps "database is locked" under WAL.
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database handle.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS items (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	url        TEXT NOT NULL,
	title      TEXT NOT NULL DEFAULT '',
	body       TEXT NOT NULL DEFAULT '',
	html       TEXT NOT NULL DEFAULT '',
	excerpt    TEXT NOT NULL DEFAULT '',
	site_name  TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS highlights (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	item_id    INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
	text       TEXT NOT NULL,
	created_at INTEGER NOT NULL
);

-- FTS5 mirror of (title, body). content-less ("external content" omitted on
-- purpose) keeps it simple; triggers keep it in lockstep with items.
CREATE VIRTUAL TABLE IF NOT EXISTS items_fts USING fts5(title, body, content='items', content_rowid='id');

CREATE TRIGGER IF NOT EXISTS items_ai AFTER INSERT ON items BEGIN
	INSERT INTO items_fts(rowid, title, body) VALUES (new.id, new.title, new.body);
END;
CREATE TRIGGER IF NOT EXISTS items_ad AFTER DELETE ON items BEGIN
	INSERT INTO items_fts(items_fts, rowid, title, body) VALUES ('delete', old.id, old.title, old.body);
END;
CREATE TRIGGER IF NOT EXISTS items_au AFTER UPDATE ON items BEGIN
	INSERT INTO items_fts(items_fts, rowid, title, body) VALUES ('delete', old.id, old.title, old.body);
	INSERT INTO items_fts(rowid, title, body) VALUES (new.id, new.title, new.body);
END;
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// AddItem inserts an item and returns it with its assigned id.
func (s *Store) AddItem(it Item) (Item, error) {
	if it.CreatedAt.IsZero() {
		it.CreatedAt = time.Now()
	}
	res, err := s.db.Exec(
		`INSERT INTO items (url, title, body, html, excerpt, site_name, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		it.URL, it.Title, it.Body, it.HTML, it.Excerpt, it.SiteName, it.CreatedAt.Unix(),
	)
	if err != nil {
		return Item{}, fmt.Errorf("insert item: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Item{}, err
	}
	it.ID = id
	return it, nil
}

// ListItems returns all items, newest first. Order is explicit (created_at DESC,
// id DESC) so output is deterministic and never relies on insertion/scan order.
func (s *Store) ListItems() ([]Item, error) {
	rows, err := s.db.Query(`
		SELECT i.id, i.url, i.title, i.excerpt, i.site_name, i.created_at, COUNT(h.id)
		FROM items i LEFT JOIN highlights h ON h.item_id = i.id
		GROUP BY i.id
		ORDER BY i.created_at DESC, i.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Item, 0)
	for rows.Next() {
		var it Item
		var ts int64
		if err := rows.Scan(&it.ID, &it.URL, &it.Title, &it.Excerpt, &it.SiteName, &ts, &it.HighlightCount); err != nil {
			return nil, err
		}
		it.CreatedAt = time.Unix(ts, 0)
		items = append(items, it)
	}
	return items, rows.Err()
}

// GetItem returns a single item (with body, html and its highlights) or
// ErrNotFound.
func (s *Store) GetItem(id int64) (Item, error) {
	var it Item
	var ts int64
	err := s.db.QueryRow(
		`SELECT id, url, title, body, html, excerpt, site_name, created_at FROM items WHERE id = ?`, id,
	).Scan(&it.ID, &it.URL, &it.Title, &it.Body, &it.HTML, &it.Excerpt, &it.SiteName, &ts)
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, err
	}
	it.CreatedAt = time.Unix(ts, 0)

	hs, err := s.highlightsFor(id)
	if err != nil {
		return Item{}, err
	}
	it.Highlights = hs
	return it, nil
}

// DeleteItem removes an item; cascades drop its highlights. Returns ErrNotFound
// if no row matched.
func (s *Store) DeleteItem(id int64) error {
	res, err := s.db.Exec(`DELETE FROM items WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Search runs an FTS5 MATCH over title+body and returns hits newest-first.
// The query is wrapped so user input is treated as a literal phrase prefix
// rather than FTS5 operator syntax (avoids parse errors on punctuation).
func (s *Store) Search(q string) ([]Item, error) {
	match := ftsQuery(q)
	if match == "" {
		return []Item{}, nil
	}
	rows, err := s.db.Query(`
		SELECT i.id, i.url, i.title, i.excerpt, i.site_name, i.created_at, COUNT(h.id)
		FROM items_fts f
		JOIN items i ON i.id = f.rowid
		LEFT JOIN highlights h ON h.item_id = i.id
		WHERE items_fts MATCH ?
		GROUP BY i.id
		ORDER BY i.created_at DESC, i.id DESC`, match)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()

	items := make([]Item, 0)
	for rows.Next() {
		var it Item
		var ts int64
		if err := rows.Scan(&it.ID, &it.URL, &it.Title, &it.Excerpt, &it.SiteName, &ts, &it.HighlightCount); err != nil {
			return nil, err
		}
		it.CreatedAt = time.Unix(ts, 0)
		items = append(items, it)
	}
	return items, rows.Err()
}

// AddHighlight attaches a passage to an item. Verifies the item exists first so
// a bad id is a clean ErrNotFound rather than a dangling foreign key.
func (s *Store) AddHighlight(itemID int64, text string) (Highlight, error) {
	if _, err := s.GetItem(itemID); err != nil {
		return Highlight{}, err
	}
	now := time.Now()
	res, err := s.db.Exec(
		`INSERT INTO highlights (item_id, text, created_at) VALUES (?, ?, ?)`, itemID, text, now.Unix())
	if err != nil {
		return Highlight{}, err
	}
	id, _ := res.LastInsertId()
	return Highlight{ID: id, ItemID: itemID, Text: text, CreatedAt: now}, nil
}

// DeleteHighlight removes a highlight, scoped to its item so a mismatched
// (item, highlight) pair can never delete a stranger's highlight. Returns
// ErrNotFound when no row matches that exact pair.
func (s *Store) DeleteHighlight(itemID, highlightID int64) error {
	res, err := s.db.Exec(`DELETE FROM highlights WHERE id = ? AND item_id = ?`, highlightID, itemID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) highlightsFor(itemID int64) ([]Highlight, error) {
	rows, err := s.db.Query(
		`SELECT id, item_id, text, created_at FROM highlights WHERE item_id = ? ORDER BY created_at ASC, id ASC`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hs := make([]Highlight, 0)
	for rows.Next() {
		var h Highlight
		var ts int64
		if err := rows.Scan(&h.ID, &h.ItemID, &h.Text, &ts); err != nil {
			return nil, err
		}
		h.CreatedAt = time.Unix(ts, 0)
		hs = append(hs, h)
	}
	return hs, rows.Err()
}
