package sqlite

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
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

// instanceConfigKey is the key used to store the singleton instance configuration
// as a JSON blob in the instance key-value table.
const instanceConfigKey = "config"

// GetInstance retrieves the singleton server instance configuration.
// Returns store.ErrServerNotFound if no instance exists.
func (s *Store) GetInstance(ctx context.Context) (*domain.Instance, error) {
	value, err := s.GetInstanceKey(ctx, instanceConfigKey)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, store.ErrServerNotFound
		}
		return nil, err
	}

	var instance domain.Instance
	if err := json.Unmarshal([]byte(value), &instance); err != nil {
		return nil, err
	}
	return &instance, nil
}

// CreateInstance creates a new singleton server instance configuration.
// Returns store.ErrServerAlreadyExists if an instance already exists.
func (s *Store) CreateInstance(ctx context.Context) (*domain.Instance, error) {
	// Check if instance already exists.
	_, err := s.GetInstance(ctx)
	if err == nil {
		return nil, store.ErrServerAlreadyExists
	}
	if !errors.Is(err, store.ErrServerNotFound) {
		return nil, err
	}

	// Create new instance with unique library ID.
	now := time.Now()
	instance := &domain.Instance{
		ID:         id.MustGenerate("lib"),
		RootUserID: "", // No root user initially - setup required
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	data, err := json.Marshal(instance)
	if err != nil {
		return nil, err
	}

	if err := s.SetInstanceKey(ctx, instanceConfigKey, string(data)); err != nil {
		return nil, err
	}

	if s.logger != nil {
		s.logger.Info("Server instance configuration created",
			"id", instance.ID,
			"setup_required", instance.IsSetupRequired(),
		)
	}

	return instance, nil
}

// UpdateInstance updates the server instance configuration.
// Returns store.ErrServerNotFound if no instance exists.
func (s *Store) UpdateInstance(ctx context.Context, instance *domain.Instance) error {
	// Verify instance exists.
	_, err := s.GetInstance(ctx)
	if err != nil {
		return err
	}

	instance.UpdatedAt = time.Now()

	data, err := json.Marshal(instance)
	if err != nil {
		return err
	}

	if err := s.SetInstanceKey(ctx, instanceConfigKey, string(data)); err != nil {
		return err
	}

	if s.logger != nil {
		s.logger.Info("Server instance configuration updated",
			"instance_id", instance.ID,
			"root_user_id", instance.RootUserID,
		)
	}

	return nil
}

// InitializeInstance ensures a server instance configuration exists.
// If no instance exists, it creates one. Returns the instance config.
func (s *Store) InitializeInstance(ctx context.Context) (*domain.Instance, error) {
	// Try to get existing instance.
	instance, err := s.GetInstance(ctx)
	if err == nil {
		if s.logger != nil {
			s.logger.Info("Server instance configuration found",
				"id", instance.ID,
				"root_user_id", instance.RootUserID,
				"setup_required", instance.IsSetupRequired(),
			)
		}
		return instance, nil
	}

	// If instance doesn't exist, create it.
	if errors.Is(err, store.ErrServerNotFound) {
		if s.logger != nil {
			s.logger.Info("No server instance configuration found, creating new instance")
		}
		return s.CreateInstance(ctx)
	}

	// Other errors.
	return nil, err
}
