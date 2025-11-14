package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

var (
	// serverKey is the singleton key for the server record.
	serverKey = []byte("server:config")

	// ErrServerNotFound is returned when no server config exists.
	ErrServerNotFound = errors.New("server not found")

	// ErrServerAlreadyExists is returned when trying to create a server that already exists.
	ErrServerAlreadyExists = errors.New("server already exists")
)

// GetInstance retrieves the singleton server instance configuration.
// Returns ErrServerNotFound if no instance exists.
func (s *Store) GetInstance(_ context.Context) (*domain.Instance, error) {
	var instance domain.Instance

	err := s.get(serverKey, &instance)
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrServerNotFound
		}
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}

	return &instance, nil
}

// CreateInstance creates a new singleton server instance configuration.
// Returns ErrServerAlreadyExists if an instance already exists.
func (s *Store) CreateInstance(_ context.Context) (*domain.Instance, error) {
	// Check if instance already exists.
	exists, err := s.exists(serverKey)
	if err != nil {
		return nil, fmt.Errorf("failed to check instance existence: %w", err)
	}

	if exists {
		return nil, ErrServerAlreadyExists
	}

	// Create new instance.
	now := time.Now()
	instance := &domain.Instance{
		ID:          "server-001", // Single server ID
		HasRootUser: false,        // No root user initially
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.set(serverKey, instance); err != nil {
		return nil, fmt.Errorf("failed to create instance: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("Server instance configuration created",
			"id", instance.ID,
			"has_root_user", instance.HasRootUser,
		)
	}

	return instance, nil
}

// UpdateInstance updates the server instance configuration.
func (s *Store) UpdateInstance(ctx context.Context, instance *domain.Instance) error {
	// Verify instance exists.
	_, err := s.GetInstance(ctx)
	if err != nil {
		return err
	}

	instance.UpdatedAt = time.Now()

	if err := s.set(serverKey, instance); err != nil {
		return fmt.Errorf("failed to update instance: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("Server instance configuration updated",
			"id", instance.ID,
			"has_root_user", instance.HasRootUser,
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
				"has_root_user", instance.HasRootUser,
				"is_setup", instance.HasRootUser,
			)
		}
		return instance, nil
	}

	// If instance doesn't exist, create it.
	if errors.Is(err, ErrServerNotFound) {
		if s.logger != nil {
			s.logger.Info("No server instance configuration found, creating new instance")
		}
		return s.CreateInstance(ctx)
	}

	// Other errors.
	return nil, fmt.Errorf("failed to initialize instance: %w", err)
}
