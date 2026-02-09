package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/domain"
	domainerrors "github.com/listenupapp/listenup-server/internal/errors"
	"github.com/listenupapp/listenup-server/internal/store"
)

// InstanceService handles business logic for server instance configuration.
type InstanceService struct {
	store  *store.Store
	logger *slog.Logger
	config *config.Config
}

// NewInstanceService creates a new instance service.
func NewInstanceService(store *store.Store, logger *slog.Logger, config *config.Config) *InstanceService {
	return &InstanceService{
		store:  store,
		logger: logger,
		config: config,
	}
}

// GetInstance retrieves the server instance configuration.
func (s *InstanceService) GetInstance(ctx context.Context) (*domain.Instance, error) {
	instance, err := s.store.GetInstance(ctx)
	if err != nil {
		if errors.Is(err, store.ErrServerNotFound) {
			return nil, domainerrors.NotFound("instance configuration not found").WithCause(err)
		}
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}

	return instance, nil
}

// InitializeInstance ensures a server instance configuration exists.
// This is the main entry point for instance setup on first run.
func (s *InstanceService) InitializeInstance(ctx context.Context) (*domain.Instance, error) {
	instance, err := s.store.InitializeInstance(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize instance: %w", err)
	}

	// Update instance with config values if they're set.
	if s.config.Server.Name != "" {
		instance.Name = s.config.Server.Name
	}
	if s.config.Server.LocalURL != "" {
		instance.LocalURL = s.config.Server.LocalURL
	}
	if s.config.Server.RemoteURL != "" {
		instance.RemoteURL = s.config.Server.RemoteURL
	}

	// Save updated instance back to store.
	if err := s.store.UpdateInstance(ctx, instance); err != nil {
		return nil, fmt.Errorf("failed to update instance with config: %w", err)
	}

	return instance, nil
}

// IsInstanceSetup checks if the server instance has been fully configured.
func (s *InstanceService) IsInstanceSetup(ctx context.Context) (bool, error) {
	instance, err := s.GetInstance(ctx)
	if err != nil {
		if errors.Is(err, store.ErrServerNotFound) {
			return false, nil
		}
		return false, err
	}

	return !instance.IsSetupRequired(), nil
}

// IsSetupRequired checks if the server requires initial setup.
// Setup is required when no root user has been configured.
func (s *InstanceService) IsSetupRequired(ctx context.Context) (bool, error) {
	instance, err := s.GetInstance(ctx)
	if err != nil {
		return false, err
	}

	return instance.IsSetupRequired(), nil
}

// SetRootUser configures the server instance with a root user.
// This should only be called once during initial setup.
func (s *InstanceService) SetRootUser(ctx context.Context, userID string) error {
	instance, err := s.GetInstance(ctx)
	if err != nil {
		return fmt.Errorf("failed to get instance: %w", err)
	}

	if !instance.IsSetupRequired() {
		return domainerrors.AlreadyConfigured("root user already configured")
	}

	instance.SetRootUser(userID)

	if err := s.store.UpdateInstance(ctx, instance); err != nil {
		return fmt.Errorf("failed to update instance: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("Root user configured",
			"instance_id", instance.ID,
			"root_user_id", userID,
		)
	}

	return nil
}

// SetOpenRegistration enables or disables public registration.
// When enabled, new users can register but require admin approval.
func (s *InstanceService) SetOpenRegistration(ctx context.Context, enabled bool) error {
	instance, err := s.GetInstance(ctx)
	if err != nil {
		return fmt.Errorf("failed to get instance: %w", err)
	}

	instance.SetOpenRegistration(enabled)

	if err := s.store.UpdateInstance(ctx, instance); err != nil {
		return fmt.Errorf("failed to update instance: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("Open registration setting changed",
			"instance_id", instance.ID,
			"enabled", enabled,
		)
	}

	return nil
}

// InstanceUpdate contains optional fields for updating instance settings.
type InstanceUpdate struct {
	Name      *string
	RemoteURL *string
}

// UpdateInstanceSettings updates mutable instance fields.
// Only non-nil fields are applied. Returns the updated instance.
func (s *InstanceService) UpdateInstanceSettings(ctx context.Context, update *InstanceUpdate) (*domain.Instance, error) {
	instance, err := s.GetInstance(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}

	if update.Name != nil {
		instance.Name = *update.Name
	}
	if update.RemoteURL != nil {
		instance.RemoteURL = *update.RemoteURL
	}
	instance.UpdatedAt = time.Now()

	if err := s.store.UpdateInstance(ctx, instance); err != nil {
		return nil, fmt.Errorf("failed to update instance: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("Instance settings updated",
			"instance_id", instance.ID,
			"name", instance.Name,
			"remote_url", instance.RemoteURL,
		)
	}

	return instance, nil
}
