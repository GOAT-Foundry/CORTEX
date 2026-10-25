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

// ProviderConfig stores a configured LLM provider profile and failover priority.
type ProviderConfig struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Format          string    `json:"format"` // "openai", "openai-responses", "anthropic"
	BaseURL         string    `json:"base_url"`
	APIKey          string    `json:"api_key,omitempty"`
	HasAPIKey       bool      `json:"has_api_key"`
	DefaultModel    string    `json:"default_model"`
	AuthType        string    `json:"auth_type"` // "copilot_device_flow", "api_key", "none"
	Enabled         bool      `json:"enabled"`
	Priority        int       `json:"priority"` // 1 = primary, 2+ = fallback order
	InputCostPer1M  float64   `json:"input_cost_per_1m"`
	OutputCostPer1M float64   `json:"output_cost_per_1m"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// UsageLogEntry represents a single model invocation's token and cost telemetry.
type UsageLogEntry struct {
	ID               int64     `json:"id"`
	ProviderID       string    `json:"provider_id"`
	Model            string    `json:"model"`
	WireFormat       string    `json:"wire_format"`
	InputTokens      int       `json:"input_tokens"`
	OutputTokens     int       `json:"output_tokens"`
	CacheReadTokens  int       `json:"cache_read_tokens"`
	CacheWriteTokens int       `json:"cache_write_tokens"`
	CostUSD          float64   `json:"cost_usd"`
	CreatedAt        time.Time `json:"created_at"`
}

// ProviderUsageBreakdown summarizes telemetry grouped by provider and model.
type ProviderUsageBreakdown struct {
	ProviderID   string  `json:"provider_id"`
	Model        string  `json:"model"`
	Calls        int     `json:"calls"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

// UsageSummary aggregates overall and per-provider token/cost statistics.
type UsageSummary struct {
	TotalCalls       int                      `json:"total_calls"`
	InputTokens      int                      `json:"input_tokens"`
	OutputTokens     int                      `json:"output_tokens"`
	CacheReadTokens  int                      `json:"cache_read_tokens"`
	CacheWriteTokens int                      `json:"cache_write_tokens"`
	TotalCostUSD     float64                  `json:"total_cost_usd"`
	ByProvider       []ProviderUsageBreakdown `json:"by_provider"`
	Recent           []UsageLogEntry          `json:"recent"`
}

// AgentProfile defines a reusable AI agent persona with its own system prompt and model preference.
type AgentProfile struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	SystemPrompt string    `json:"system_prompt"`
	ProviderID   string    `json:"provider_id"`
	Model        string    `json:"model"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ChatSession represents a persistent multi-turn conversation thread.
type ChatSession struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	AgentID    string    `json:"agent_id"`
	ProviderID string    `json:"provider_id"`
	Model      string    `json:"model"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ChatToolCall records a single tool invocation and its outcome within an assistant turn.
type ChatToolCall struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Input   string `json:"input,omitempty"`
	Output  string `json:"output,omitempty"`
	IsError bool   `json:"is_error,omitempty"`
}

// ChatAttachment represents a file or image attached to a user message.
type ChatAttachment struct {
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	Content   string `json:"content"` // UTF-8 text content or base64 data URI
	SizeBytes int    `json:"size_bytes,omitempty"`
}

// ChatMessage represents one user or assistant message in a ChatSession.
type ChatMessage struct {
	ID           int64            `json:"id"`
	SessionID    string           `json:"session_id"`
	Role         string           `json:"role"` // "user" | "assistant"
	Content      string           `json:"content"`
	Thinking     string           `json:"thinking,omitempty"`
	ToolCalls    []ChatToolCall   `json:"tool_calls,omitempty"`
	Attachments  []ChatAttachment `json:"attachments,omitempty"`
	InputTokens  int              `json:"input_tokens,omitempty"`
	OutputTokens int              `json:"output_tokens,omitempty"`
	CreatedAt    time.Time        `json:"created_at"`
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

	CREATE TABLE IF NOT EXISTS provider_configs (
		id                 TEXT PRIMARY KEY,
		name               TEXT NOT NULL,
		format             TEXT NOT NULL,
		base_url           TEXT NOT NULL,
		api_key            TEXT NOT NULL DEFAULT '',
		default_model      TEXT NOT NULL,
		auth_type          TEXT NOT NULL DEFAULT 'api_key',
		enabled            INTEGER NOT NULL DEFAULT 1,
		priority           INTEGER NOT NULL DEFAULT 10,
		input_cost_per_1m  REAL NOT NULL DEFAULT 0,
		output_cost_per_1m REAL NOT NULL DEFAULT 0,
		updated_at         DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS usage_logs (
		id                 INTEGER PRIMARY KEY AUTOINCREMENT,
		provider_id        TEXT NOT NULL,
		model              TEXT NOT NULL,
		wire_format        TEXT NOT NULL,
		input_tokens       INTEGER NOT NULL DEFAULT 0,
		output_tokens      INTEGER NOT NULL DEFAULT 0,
		cache_read_tokens  INTEGER NOT NULL DEFAULT 0,
		cache_write_tokens INTEGER NOT NULL DEFAULT 0,
		cost_usd           REAL NOT NULL DEFAULT 0,
		created_at         DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_usage_logs_created ON usage_logs(created_at DESC);

	CREATE TABLE IF NOT EXISTS agents (
		id            TEXT PRIMARY KEY,
		name          TEXT NOT NULL,
		description   TEXT NOT NULL DEFAULT '',
		system_prompt TEXT NOT NULL,
		provider_id   TEXT NOT NULL DEFAULT '',
		model         TEXT NOT NULL DEFAULT '',
		created_at    DATETIME NOT NULL,
		updated_at    DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS chat_sessions (
		id          TEXT PRIMARY KEY,
		title       TEXT NOT NULL,
		agent_id    TEXT NOT NULL DEFAULT 'default',
		provider_id TEXT NOT NULL DEFAULT '',
		model       TEXT NOT NULL DEFAULT '',
		created_at  DATETIME NOT NULL,
		updated_at  DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_chat_sessions_updated ON chat_sessions(updated_at DESC);

	CREATE TABLE IF NOT EXISTS chat_messages (
		id               INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id       TEXT NOT NULL,
		role             TEXT NOT NULL,
		content          TEXT NOT NULL,
		thinking         TEXT NOT NULL DEFAULT '',
		tool_calls_json  TEXT NOT NULL DEFAULT '[]',
		attachments_json TEXT NOT NULL DEFAULT '[]',
		input_tokens     INTEGER NOT NULL DEFAULT 0,
		output_tokens    INTEGER NOT NULL DEFAULT 0,
		created_at       DATETIME NOT NULL,
		FOREIGN KEY (session_id) REFERENCES chat_sessions(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_chat_messages_session ON chat_messages(session_id, id ASC);
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

// DeleteSetting removes a key from the settings table.
func (s *Store) DeleteSetting(ctx context.Context, key string) error {
	if err := ValidateIdent(key); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, key)
	return err
}

// ListProviderConfigs returns all provider configurations ordered by priority ASC, id ASC.
func (s *Store) ListProviderConfigs(ctx context.Context) ([]ProviderConfig, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, format, base_url, api_key, default_model, auth_type,
		       enabled, priority, input_cost_per_1m, output_cost_per_1m, updated_at
		FROM provider_configs
		ORDER BY priority ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ProviderConfig{}
	for rows.Next() {
		var p ProviderConfig
		var enabled int
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Format, &p.BaseURL, &p.APIKey, &p.DefaultModel, &p.AuthType,
			&enabled, &p.Priority, &p.InputCostPer1M, &p.OutputCostPer1M, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		p.Enabled = enabled != 0
		p.HasAPIKey = p.APIKey != ""
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetProviderConfig retrieves a single provider configuration by ID.
func (s *Store) GetProviderConfig(ctx context.Context, id string) (ProviderConfig, error) {
	if err := ValidateIdent(id); err != nil {
		return ProviderConfig{}, err
	}
	var p ProviderConfig
	var enabled int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, format, base_url, api_key, default_model, auth_type,
		       enabled, priority, input_cost_per_1m, output_cost_per_1m, updated_at
		FROM provider_configs
		WHERE id = ?
	`, id).Scan(
		&p.ID, &p.Name, &p.Format, &p.BaseURL, &p.APIKey, &p.DefaultModel, &p.AuthType,
		&enabled, &p.Priority, &p.InputCostPer1M, &p.OutputCostPer1M, &p.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ProviderConfig{}, ErrNotFound
	}
	if err != nil {
		return ProviderConfig{}, err
	}
	p.Enabled = enabled != 0
	p.HasAPIKey = p.APIKey != ""
	return p, nil
}

// SaveProviderConfig inserts or updates a provider configuration.
// If p.APIKey is empty and a record already exists, the existing api_key is preserved
// unless clearAPIKey is true.
func (s *Store) SaveProviderConfig(ctx context.Context, p ProviderConfig, clearAPIKey bool) (ProviderConfig, error) {
	if err := ValidateIdent(p.ID); err != nil {
		return ProviderConfig{}, err
	}
	if !clearAPIKey && p.APIKey == "" {
		if existing, err := s.GetProviderConfig(ctx, p.ID); err == nil {
			p.APIKey = existing.APIKey
		}
	}
	now := time.Now().UTC()
	p.UpdatedAt = now
	enabled := 0
	if p.Enabled {
		enabled = 1
	}
	if p.Priority <= 0 {
		p.Priority = 10
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO provider_configs (
			id, name, format, base_url, api_key, default_model, auth_type,
			enabled, priority, input_cost_per_1m, output_cost_per_1m, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			format = excluded.format,
			base_url = excluded.base_url,
			api_key = excluded.api_key,
			default_model = excluded.default_model,
			auth_type = excluded.auth_type,
			enabled = excluded.enabled,
			priority = excluded.priority,
			input_cost_per_1m = excluded.input_cost_per_1m,
			output_cost_per_1m = excluded.output_cost_per_1m,
			updated_at = excluded.updated_at
	`,
		p.ID, p.Name, p.Format, p.BaseURL, p.APIKey, p.DefaultModel, p.AuthType,
		enabled, p.Priority, p.InputCostPer1M, p.OutputCostPer1M, p.UpdatedAt,
	)
	if err != nil {
		return ProviderConfig{}, err
	}
	return s.GetProviderConfig(ctx, p.ID)
}

// RecordUsage appends a token/cost telemetry entry to usage_logs.
func (s *Store) RecordUsage(ctx context.Context, entry UsageLogEntry) (UsageLogEntry, error) {
	now := time.Now().UTC()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO usage_logs (
			provider_id, model, wire_format,
			input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
			cost_usd, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		entry.ProviderID, entry.Model, entry.WireFormat,
		entry.InputTokens, entry.OutputTokens, entry.CacheReadTokens, entry.CacheWriteTokens,
		entry.CostUSD, entry.CreatedAt,
	)
	if err != nil {
		return UsageLogEntry{}, err
	}
	id, _ := res.LastInsertId()
	entry.ID = id
	return entry, nil
}

// GetUsageSummary aggregates total and per-provider token/cost usage and returns recent logs.
func (s *Store) GetUsageSummary(ctx context.Context, limitRecent int) (UsageSummary, error) {
	if limitRecent <= 0 {
		limitRecent = 20
	}
	var sum UsageSummary
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*),
		       COALESCE(SUM(input_tokens), 0),
		       COALESCE(SUM(output_tokens), 0),
		       COALESCE(SUM(cache_read_tokens), 0),
		       COALESCE(SUM(cache_write_tokens), 0),
		       COALESCE(SUM(cost_usd), 0)
		FROM usage_logs
	`).Scan(
		&sum.TotalCalls,
		&sum.InputTokens,
		&sum.OutputTokens,
		&sum.CacheReadTokens,
		&sum.CacheWriteTokens,
		&sum.TotalCostUSD,
	)
	if err != nil {
		return UsageSummary{}, err
	}

	bRows, err := s.db.QueryContext(ctx, `
		SELECT provider_id, model, COUNT(*),
		       COALESCE(SUM(input_tokens), 0),
		       COALESCE(SUM(output_tokens), 0),
		       COALESCE(SUM(cost_usd), 0)
		FROM usage_logs
		GROUP BY provider_id, model
		ORDER BY SUM(cost_usd) DESC, COUNT(*) DESC
	`)
	if err != nil {
		return UsageSummary{}, err
	}
	defer bRows.Close()

	sum.ByProvider = []ProviderUsageBreakdown{}
	for bRows.Next() {
		var b ProviderUsageBreakdown
		if err := bRows.Scan(&b.ProviderID, &b.Model, &b.Calls, &b.InputTokens, &b.OutputTokens, &b.CostUSD); err != nil {
			return UsageSummary{}, err
		}
		sum.ByProvider = append(sum.ByProvider, b)
	}
	if err := bRows.Err(); err != nil {
		return UsageSummary{}, err
	}

	rRows, err := s.db.QueryContext(ctx, `
		SELECT id, provider_id, model, wire_format,
		       input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
		       cost_usd, created_at
		FROM usage_logs
		ORDER BY id DESC
		LIMIT ?
	`, limitRecent)
	if err != nil {
		return UsageSummary{}, err
	}
	defer rRows.Close()

	sum.Recent = []UsageLogEntry{}
	for rRows.Next() {
		var e UsageLogEntry
		if err := rRows.Scan(
			&e.ID, &e.ProviderID, &e.Model, &e.WireFormat,
			&e.InputTokens, &e.OutputTokens, &e.CacheReadTokens, &e.CacheWriteTokens,
			&e.CostUSD, &e.CreatedAt,
		); err != nil {
			return UsageSummary{}, err
		}
		sum.Recent = append(sum.Recent, e)
	}
	return sum, rRows.Err()
}

// ListAgents returns all registered agent profiles ordered by id ASC.
func (s *Store) ListAgents(ctx context.Context) ([]AgentProfile, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description, system_prompt, provider_id, model, created_at, updated_at
		FROM agents
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []AgentProfile{}
	for rows.Next() {
		var a AgentProfile
		if err := rows.Scan(&a.ID, &a.Name, &a.Description, &a.SystemPrompt, &a.ProviderID, &a.Model, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetAgent fetches a single agent profile by ID.
func (s *Store) GetAgent(ctx context.Context, id string) (AgentProfile, error) {
	if err := ValidateIdent(id); err != nil {
		return AgentProfile{}, err
	}
	var a AgentProfile
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, system_prompt, provider_id, model, created_at, updated_at
		FROM agents
		WHERE id = ?
	`, id).Scan(&a.ID, &a.Name, &a.Description, &a.SystemPrompt, &a.ProviderID, &a.Model, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentProfile{}, ErrNotFound
	}
	return a, err
}

// SaveAgent creates or updates an agent profile.
func (s *Store) SaveAgent(ctx context.Context, a AgentProfile) (AgentProfile, error) {
	if err := ValidateIdent(a.ID); err != nil {
		return AgentProfile{}, err
	}
	now := time.Now().UTC()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agents (id, name, description, system_prompt, provider_id, model, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			system_prompt = excluded.system_prompt,
			provider_id = excluded.provider_id,
			model = excluded.model,
			updated_at = excluded.updated_at
	`, a.ID, a.Name, a.Description, a.SystemPrompt, a.ProviderID, a.Model, a.CreatedAt, a.UpdatedAt)
	if err != nil {
		return AgentProfile{}, err
	}
	return s.GetAgent(ctx, a.ID)
}

// ListChatSessions returns all chat sessions ordered by most recently updated.
func (s *Store) ListChatSessions(ctx context.Context) ([]ChatSession, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, agent_id, provider_id, model, created_at, updated_at
		FROM chat_sessions
		ORDER BY updated_at DESC, created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ChatSession{}
	for rows.Next() {
		var cs ChatSession
		if err := rows.Scan(&cs.ID, &cs.Title, &cs.AgentID, &cs.ProviderID, &cs.Model, &cs.CreatedAt, &cs.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, cs)
	}
	return out, rows.Err()
}

// GetChatSession retrieves a single chat session by ID.
func (s *Store) GetChatSession(ctx context.Context, id string) (ChatSession, error) {
	if err := ValidateIdent(id); err != nil {
		return ChatSession{}, err
	}
	var cs ChatSession
	err := s.db.QueryRowContext(ctx, `
		SELECT id, title, agent_id, provider_id, model, created_at, updated_at
		FROM chat_sessions
		WHERE id = ?
	`, id).Scan(&cs.ID, &cs.Title, &cs.AgentID, &cs.ProviderID, &cs.Model, &cs.CreatedAt, &cs.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ChatSession{}, ErrNotFound
	}
	return cs, err
}

// SaveChatSession creates or updates a chat session.
func (s *Store) SaveChatSession(ctx context.Context, cs ChatSession) (ChatSession, error) {
	if err := ValidateIdent(cs.ID); err != nil {
		return ChatSession{}, err
	}
	now := time.Now().UTC()
	if cs.CreatedAt.IsZero() {
		cs.CreatedAt = now
	}
	cs.UpdatedAt = now
	if cs.AgentID == "" {
		cs.AgentID = "default"
	}
	if cs.Title == "" {
		cs.Title = "New Chat"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO chat_sessions (id, title, agent_id, provider_id, model, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title,
			agent_id = excluded.agent_id,
			provider_id = excluded.provider_id,
			model = excluded.model,
			updated_at = excluded.updated_at
	`, cs.ID, cs.Title, cs.AgentID, cs.ProviderID, cs.Model, cs.CreatedAt, cs.UpdatedAt)
	if err != nil {
		return ChatSession{}, err
	}
	return s.GetChatSession(ctx, cs.ID)
}

// DeleteChatSession deletes a chat session and its messages.
func (s *Store) DeleteChatSession(ctx context.Context, id string) error {
	if err := ValidateIdent(id); err != nil {
		return err
	}
	_, _ = s.db.ExecContext(ctx, `DELETE FROM chat_messages WHERE session_id = ?`, id)
	res, err := s.db.ExecContext(ctx, `DELETE FROM chat_sessions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// AppendChatMessage appends a message to a chat session and updates the session's updated_at timestamp.
func (s *Store) AppendChatMessage(ctx context.Context, m ChatMessage) (ChatMessage, error) {
	if err := ValidateIdent(m.SessionID); err != nil {
		return ChatMessage{}, err
	}
	now := time.Now().UTC()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	tcJSON := "[]"
	if len(m.ToolCalls) > 0 {
		if b, err := json.Marshal(m.ToolCalls); err == nil {
			tcJSON = string(b)
		}
	}
	attJSON := "[]"
	if len(m.Attachments) > 0 {
		if b, err := json.Marshal(m.Attachments); err == nil {
			attJSON = string(b)
		}
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO chat_messages (
			session_id, role, content, thinking, tool_calls_json, attachments_json,
			input_tokens, output_tokens, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, m.SessionID, m.Role, m.Content, m.Thinking, tcJSON, attJSON, m.InputTokens, m.OutputTokens, m.CreatedAt)
	if err != nil {
		return ChatMessage{}, err
	}
	id, _ := res.LastInsertId()
	m.ID = id
	_, _ = s.db.ExecContext(ctx, `UPDATE chat_sessions SET updated_at = ? WHERE id = ?`, now, m.SessionID)
	return m, nil
}

// ListChatMessages returns all messages for a chat session ordered chronologically.
func (s *Store) ListChatMessages(ctx context.Context, sessionID string) ([]ChatMessage, error) {
	if err := ValidateIdent(sessionID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_id, role, content, thinking, tool_calls_json, attachments_json,
		       input_tokens, output_tokens, created_at
		FROM chat_messages
		WHERE session_id = ?
		ORDER BY id ASC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ChatMessage{}
	for rows.Next() {
		var m ChatMessage
		var tcRaw, attRaw string
		if err := rows.Scan(
			&m.ID, &m.SessionID, &m.Role, &m.Content, &m.Thinking,
			&tcRaw, &attRaw, &m.InputTokens, &m.OutputTokens, &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(tcRaw), &m.ToolCalls)
		_ = json.Unmarshal([]byte(attRaw), &m.Attachments)
		out = append(out, m)
	}
	return out, rows.Err()
}

