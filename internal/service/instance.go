package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// InstanceService handles business logic for server instance configuration.
type InstanceService struct {
	store  *store.Store
	logger *slog.Logger
	config *config.Config
}

// NewInstanceService creates a new instance service
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
			return nil, fmt.Errorf("instance configuration not found: %w", err)
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

	instance = &domain.Instance{
		ID:        uuid.New().String(),
		Name:      s.config.Server.Name,
		Version:   "0.1.0",
		LocalUrl:  s.config.Server.LocalURL,
		RemoteUrl: s.config.Server.RemoteURL,
		CreatedAt: time.Now(),
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

	return instance.HasRootUser, nil
}

// MarkInstanceAsSetup marks the server instance as fully configured with a root user.
// This would typically be called after root user creation.
func (s *InstanceService) MarkInstanceAsSetup(ctx context.Context) error {
	instance, err := s.store.GetInstance(ctx)
	if err != nil {
		return fmt.Errorf("failed to get instance: %w", err)
	}

	if instance.HasRootUser {
		return fmt.Errorf("instance is already set up")
	}

	instance.HasRootUser = true

	if err := s.store.UpdateInstance(ctx, instance); err != nil {
		return fmt.Errorf("failed to mark instance as setup: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("Server instance marked as setup", "instance_id", instance.ID)
	}

	return nil
}
