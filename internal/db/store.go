package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS oauth_tokens (
    user_id    TEXT NOT NULL,
    provider   TEXT NOT NULL,
    access_token  TEXT NOT NULL,
    refresh_token TEXT,
    expires_at TEXT,
    api_server TEXT,
    PRIMARY KEY (user_id, provider),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS mappings (
    id                       TEXT PRIMARY KEY,
    user_id                  TEXT NOT NULL,
    questrade_account_number TEXT NOT NULL,
    ynab_budget_id           TEXT NOT NULL,
    ynab_account_id          TEXT NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS sync_schedules (
    user_id         TEXT PRIMARY KEY,
    cron_expression TEXT NOT NULL DEFAULT '0 9 * * *',
    enabled         INTEGER NOT NULL DEFAULT 0,
    next_run_at     TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS sync_history (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL,
    ran_at          TEXT NOT NULL DEFAULT (datetime('now')),
    status          TEXT NOT NULL,
    accounts_synced INTEGER NOT NULL DEFAULT 0,
    detail          TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
`

// SQLiteStore is the local, file-backed implementation of Store.
type SQLiteStore struct {
	db *sql.DB
}

// --- models ---

type OAuthToken struct {
	UserID       string
	Provider     string
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	APIServer    string // Questrade only
}

type Mapping struct {
	ID                     string `json:"id"`
	UserID                 string `json:"user_id"`
	QuestradeAccountNumber string `json:"questrade_account_number"`
	YNABBudgetID           string `json:"ynab_budget_id"`
	YNABAccountID          string `json:"ynab_account_id"`
}

type SyncSchedule struct {
	UserID         string     `json:"user_id"`
	CronExpression string     `json:"cron_expression"`
	Enabled        bool       `json:"enabled"`
	NextRunAt      *time.Time `json:"next_run_at"`
}

type SyncHistory struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	RanAt          time.Time `json:"ran_at"`
	Status         string    `json:"status"`
	AccountsSynced int       `json:"accounts_synced"`
	Detail         string    `json:"detail"` // JSON string
}

// --- open ---

// OpenSQLite opens (creating the parent dir and schema if needed) a file-backed
// SQLite store. Used for local development.
func OpenSQLite(path string) (*SQLiteStore, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create data dir: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite is single-writer
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// --- users ---

func (s *SQLiteStore) CreateUser() (string, error) {
	id := uuid.New().String()
	_, err := s.db.Exec(`INSERT INTO users (id) VALUES (?)`, id)
	if err != nil {
		return "", fmt.Errorf("create user: %w", err)
	}
	return id, nil
}

func (s *SQLiteStore) UserExists(id string) (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ?`, id).Scan(&count)
	return count > 0, err
}

// --- oauth tokens ---

func (s *SQLiteStore) UpsertToken(t OAuthToken) error {
	_, err := s.db.Exec(`
		INSERT INTO oauth_tokens (user_id, provider, access_token, refresh_token, expires_at, api_server)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, provider) DO UPDATE SET
			access_token  = excluded.access_token,
			refresh_token = excluded.refresh_token,
			expires_at    = excluded.expires_at,
			api_server    = excluded.api_server
	`,
		t.UserID, t.Provider, t.AccessToken, t.RefreshToken,
		t.ExpiresAt.UTC().Format(time.RFC3339), t.APIServer,
	)
	return err
}

func (s *SQLiteStore) GetToken(userID, provider string) (*OAuthToken, error) {
	var t OAuthToken
	var expiresAt string
	err := s.db.QueryRow(`
		SELECT user_id, provider, access_token, refresh_token, expires_at, api_server
		FROM oauth_tokens WHERE user_id = ? AND provider = ?
	`, userID, provider).Scan(
		&t.UserID, &t.Provider, &t.AccessToken, &t.RefreshToken, &expiresAt, &t.APIServer,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)
	return &t, nil
}

func (s *SQLiteStore) HasToken(userID, provider string) (bool, error) {
	t, err := s.GetToken(userID, provider)
	return t != nil, err
}

// --- mappings ---

func (s *SQLiteStore) GetMappings(userID string) ([]Mapping, error) {
	rows, err := s.db.Query(`
		SELECT id, user_id, questrade_account_number, ynab_budget_id, ynab_account_id
		FROM mappings WHERE user_id = ?
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Mapping
	for rows.Next() {
		var m Mapping
		if err := rows.Scan(&m.ID, &m.UserID, &m.QuestradeAccountNumber, &m.YNABBudgetID, &m.YNABAccountID); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ReplaceMappings deletes all mappings for the user and inserts new ones atomically.
func (s *SQLiteStore) ReplaceMappings(userID string, mappings []Mapping) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM mappings WHERE user_id = ?`, userID); err != nil {
		return err
	}
	for _, m := range mappings {
		id := uuid.New().String()
		if _, err := tx.Exec(`
			INSERT INTO mappings (id, user_id, questrade_account_number, ynab_budget_id, ynab_account_id)
			VALUES (?, ?, ?, ?, ?)
		`, id, userID, m.QuestradeAccountNumber, m.YNABBudgetID, m.YNABAccountID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) DeleteMapping(userID, mappingID string) error {
	_, err := s.db.Exec(`DELETE FROM mappings WHERE id = ? AND user_id = ?`, mappingID, userID)
	return err
}

// --- sync schedule ---

func (s *SQLiteStore) GetSchedule(userID string) (*SyncSchedule, error) {
	var sc SyncSchedule
	var nextRunAt sql.NullString
	err := s.db.QueryRow(`
		SELECT user_id, cron_expression, enabled, next_run_at
		FROM sync_schedules WHERE user_id = ?
	`, userID).Scan(&sc.UserID, &sc.CronExpression, &sc.Enabled, &nextRunAt)
	if err == sql.ErrNoRows {
		return &SyncSchedule{UserID: userID, CronExpression: "0 9 * * *", Enabled: false}, nil
	}
	if err != nil {
		return nil, err
	}
	if nextRunAt.Valid {
		t, _ := time.Parse(time.RFC3339, nextRunAt.String)
		sc.NextRunAt = &t
	}
	return &sc, nil
}

func (s *SQLiteStore) UpsertSchedule(sc SyncSchedule) error {
	var nextRunAt *string
	if sc.NextRunAt != nil {
		s := sc.NextRunAt.UTC().Format(time.RFC3339)
		nextRunAt = &s
	}
	enabled := 0
	if sc.Enabled {
		enabled = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO sync_schedules (user_id, cron_expression, enabled, next_run_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			cron_expression = excluded.cron_expression,
			enabled         = excluded.enabled,
			next_run_at     = excluded.next_run_at
	`, sc.UserID, sc.CronExpression, enabled, nextRunAt)
	return err
}

func (s *SQLiteStore) GetAllEnabledSchedules() ([]SyncSchedule, error) {
	rows, err := s.db.Query(`
		SELECT user_id, cron_expression, enabled, next_run_at
		FROM sync_schedules WHERE enabled = 1
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SyncSchedule
	for rows.Next() {
		var sc SyncSchedule
		var nextRunAt sql.NullString
		if err := rows.Scan(&sc.UserID, &sc.CronExpression, &sc.Enabled, &nextRunAt); err != nil {
			return nil, err
		}
		if nextRunAt.Valid {
			t, _ := time.Parse(time.RFC3339, nextRunAt.String)
			sc.NextRunAt = &t
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

// --- sync history ---

func (s *SQLiteStore) CreateSyncHistory(h SyncHistory) error {
	id := uuid.New().String()
	_, err := s.db.Exec(`
		INSERT INTO sync_history (id, user_id, ran_at, status, accounts_synced, detail)
		VALUES (?, ?, ?, ?, ?, ?)
	`, id, h.UserID, h.RanAt.UTC().Format(time.RFC3339), h.Status, h.AccountsSynced, h.Detail)
	return err
}

func (s *SQLiteStore) GetSyncHistory(userID string, limit int) ([]SyncHistory, error) {
	rows, err := s.db.Query(`
		SELECT id, user_id, ran_at, status, accounts_synced, detail
		FROM sync_history WHERE user_id = ?
		ORDER BY ran_at DESC LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SyncHistory
	for rows.Next() {
		var h SyncHistory
		var ranAt string
		if err := rows.Scan(&h.ID, &h.UserID, &ranAt, &h.Status, &h.AccountsSynced, &h.Detail); err != nil {
			return nil, err
		}
		h.RanAt, _ = time.Parse(time.RFC3339, ranAt)
		out = append(out, h)
	}
	return out, rows.Err()
}
