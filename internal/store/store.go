package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	_ "modernc.org/sqlite"
)

var (
	// ErrNotFound indicates a requested record does not exist.
	ErrNotFound = errors.New("store: record not found")
	// ErrInvalidName indicates an invalid collection or identifier name.
	ErrInvalidName = errors.New("store: invalid identifier")

	validIdentRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
)

// CollectionItem represents a schema-free JSON document stored in a named collection.
type CollectionItem struct {
	Collection string          `json:"collection"`
	ID         string          `json:"id"`
	Data       json.RawMessage `json:"data"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// TimerEntry represents a scheduled or one-shot timer task.
type TimerEntry struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Schedule   string    `json:"schedule"` // cron expression, interval, or "once"
	RunAt      string    `json:"run_at,omitempty"`
	Prompt     string    `json:"prompt"`
	AgentID    string    `json:"agent_id"`
	Enabled    bool      `json:"enabled"`
	CreatedBy  string    `json:"created_by"` // "user" or "agent"
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Store wraps SQLite persistence for CORTEX collections, timers, and settings.
type Store struct {
	db *sql.DB
}

// Open initializes a SQLite-backed Store at dbPath (or ":memory:").
func Open(dbPath string) (*Store, error) {
	if dbPath != ":memory:" {
		dir := filepath.Dir(dbPath)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("store: mkdir %s: %w", dir, err)
			}
		}
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("store: open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	schema := `
	PRAGMA journal_mode = WAL;
	PRAGMA foreign_keys = ON;

	CREATE TABLE IF NOT EXISTS collections (
		collection TEXT NOT NULL,
		id         TEXT NOT NULL,
		data       TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		PRIMARY KEY (collection, id)
	);

	CREATE INDEX IF NOT EXISTS idx_collections_name ON collections(collection, updated_at DESC);

	CREATE TABLE IF NOT EXISTS timers (
		id         TEXT PRIMARY KEY,
		name       TEXT NOT NULL,
		schedule   TEXT NOT NULL,
		run_at     TEXT NOT NULL DEFAULT '',
		prompt     TEXT NOT NULL,
		agent_id   TEXT NOT NULL DEFAULT 'default',
		enabled    INTEGER NOT NULL DEFAULT 1,
		created_by TEXT NOT NULL DEFAULT 'user',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS settings (
		key        TEXT PRIMARY KEY,
		value      TEXT NOT NULL,
		updated_at DATETIME NOT NULL
	);
	`
	_, err := s.db.Exec(schema)
	return err
}

// ValidateIdent checks if a collection name or item ID is safe.
func ValidateIdent(name string) error {
	if !validIdentRe.MatchString(name) {
		return fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	return nil
}

// ListCollections returns distinct collection names currently stored.
func (s *Store) ListCollections(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT collection FROM collections ORDER BY collection ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// ListItems returns all documents in a collection ordered by most recently updated.
func (s *Store) ListItems(ctx context.Context, collection string) ([]CollectionItem, error) {
	if err := ValidateIdent(collection); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT collection, id, data, created_at, updated_at FROM collections WHERE collection = ? ORDER BY updated_at DESC`,
		collection,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []CollectionItem{}
	for rows.Next() {
		var it CollectionItem
		var raw string
		if err := rows.Scan(&it.Collection, &it.ID, &raw, &it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		it.Data = json.RawMessage(raw)
		items = append(items, it)
	}
	return items, rows.Err()
}

// GetItem fetches a single document by collection and ID.
func (s *Store) GetItem(ctx context.Context, collection, id string) (CollectionItem, error) {
	if err := ValidateIdent(collection); err != nil {
		return CollectionItem{}, err
	}
	if err := ValidateIdent(id); err != nil {
		return CollectionItem{}, err
	}
	var it CollectionItem
	var raw string
	err := s.db.QueryRowContext(ctx,
		`SELECT collection, id, data, created_at, updated_at FROM collections WHERE collection = ? AND id = ?`,
		collection, id,
	).Scan(&it.Collection, &it.ID, &raw, &it.CreatedAt, &it.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CollectionItem{}, ErrNotFound
	}
	if err != nil {
		return CollectionItem{}, err
	}
	it.Data = json.RawMessage(raw)
	return it, nil
}

// PutItem creates or updates a JSON document in a collection.
func (s *Store) PutItem(ctx context.Context, collection, id string, data json.RawMessage) (CollectionItem, error) {
	if err := ValidateIdent(collection); err != nil {
		return CollectionItem{}, err
	}
	if err := ValidateIdent(id); err != nil {
		return CollectionItem{}, err
	}
	if !json.Valid(data) {
		return CollectionItem{}, errors.New("store: payload is not valid JSON")
	}

	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO collections (collection, id, data, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(collection, id) DO UPDATE SET
			data = excluded.data,
			updated_at = excluded.updated_at
	`, collection, id, string(data), now, now)
	if err != nil {
		return CollectionItem{}, err
	}
	return s.GetItem(ctx, collection, id)
}

// DeleteItem removes a document from a collection.
func (s *Store) DeleteItem(ctx context.Context, collection, id string) error {
	if err := ValidateIdent(collection); err != nil {
		return err
	}
	if err := ValidateIdent(id); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM collections WHERE collection = ? AND id = ?`, collection, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListTimers returns all registered timer entries.
func (s *Store) ListTimers(ctx context.Context) ([]TimerEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, schedule, run_at, prompt, agent_id, enabled, created_by, created_at, updated_at FROM timers ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []TimerEntry{}
	for rows.Next() {
		var t TimerEntry
		var enabled int
		if err := rows.Scan(&t.ID, &t.Name, &t.Schedule, &t.RunAt, &t.Prompt, &t.AgentID, &enabled, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.Enabled = enabled != 0
		out = append(out, t)
	}
	return out, rows.Err()
}

// SaveTimer inserts or updates a timer entry.
func (s *Store) SaveTimer(ctx context.Context, t TimerEntry) (TimerEntry, error) {
	if err := ValidateIdent(t.ID); err != nil {
		return TimerEntry{}, err
	}
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	if t.AgentID == "" {
		t.AgentID = "default"
	}
	if t.CreatedBy == "" {
		t.CreatedBy = "user"
	}
	enabled := 0
	if t.Enabled {
		enabled = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO timers (id, name, schedule, run_at, prompt, agent_id, enabled, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			schedule = excluded.schedule,
			run_at = excluded.run_at,
			prompt = excluded.prompt,
			agent_id = excluded.agent_id,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at
	`, t.ID, t.Name, t.Schedule, t.RunAt, t.Prompt, t.AgentID, enabled, t.CreatedBy, t.CreatedAt, t.UpdatedAt)
	return t, err
}

// GetSetting retrieves a persisted key-value setting from SQLite.
func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	if err := ValidateIdent(key); err != nil {
		return "", err
	}
	var val string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&val)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return val, nil
}

// SetSetting persists a key-value setting in SQLite.
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	if err := ValidateIdent(key); err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at
	`, key, value, now)
	return err
}

