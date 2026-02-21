package sqlite

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// GetInstanceKey retrieves a value from the instance key-value table.
// Returns store.ErrNotFound if the key does not exist.
func (s *Store) GetInstanceKey(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx,
		`SELECT value FROM instance WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", store.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

// SetInstanceKey sets a value in the instance key-value table.
// Creates the key if it does not exist, or replaces the existing value.
func (s *Store) SetInstanceKey(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO instance (key, value) VALUES (?, ?)`, key, value)
	return err
}

// GetServerSettings retrieves server-wide settings.
// Returns default settings (via domain.NewServerSettings()) if no settings exist.
func (s *Store) GetServerSettings(ctx context.Context) (*domain.ServerSettings, error) {
	var value string
	err := s.db.QueryRowContext(ctx,
		`SELECT value FROM server_settings WHERE key = 'server'`).Scan(&value)
	if err == sql.ErrNoRows {
		return domain.NewServerSettings(), nil
	}
	if err != nil {
		return nil, err
	}

	var settings domain.ServerSettings
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return nil, err
	}
	return &settings, nil
}

// UpdateServerSettings persists server-wide settings.
// Creates the record if it does not exist, or replaces the existing value.
func (s *Store) UpdateServerSettings(ctx context.Context, settings *domain.ServerSettings) error {
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO server_settings (key, value, updated_at) VALUES ('server', ?, ?)`,
		string(data),
		formatTime(time.Now().UTC()),
	)
	return err
}
