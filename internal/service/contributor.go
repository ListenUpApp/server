package service

import (
	"context"
	"log/slog"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/search/asyncindexer"
	"github.com/listenupapp/listenup-server/internal/store"
)

// contributorServiceStore is the narrow store interface ContributorService depends on.
type contributorServiceStore interface {
	store.ContributorStore
	// ACL helpers used by read paths in the handlers.
	GetAccessibleBookIDSet(ctx context.Context, userID string) (map[string]bool, error)
}

// ContributorService coordinates contributor CRUD with search indexing.
type ContributorService struct {
	store   contributorServiceStore
	indexer *asyncindexer.Indexer
	logger  *slog.Logger
}

// NewContributorService creates a new ContributorService.
func NewContributorService(s contributorServiceStore, indexer *asyncindexer.Indexer, logger *slog.Logger) *ContributorService {
	return &ContributorService{store: s, indexer: indexer, logger: logger}
}

// ListContributors returns a paginated list of contributors.
func (s *ContributorService) ListContributors(ctx context.Context, params store.PaginationParams) (*store.PaginatedResult[*domain.Contributor], error) {
	return s.store.ListContributors(ctx, params)
}

// GetContributor returns a contributor by ID.
func (s *ContributorService) GetContributor(ctx context.Context, id string) (*domain.Contributor, error) {
	return s.store.GetContributor(ctx, id)
}

// CreateContributor persists a new contributor and enqueues an index update.
func (s *ContributorService) CreateContributor(ctx context.Context, c *domain.Contributor) error {
	if err := s.store.CreateContributor(ctx, c); err != nil {
		return err
	}
	s.indexer.SubmitIndexContributor(c)
	return nil
}

// UpdateContributor persists changes to an existing contributor and enqueues an index update.
func (s *ContributorService) UpdateContributor(ctx context.Context, c *domain.Contributor) error {
	if err := s.store.UpdateContributor(ctx, c); err != nil {
		return err
	}
	s.indexer.SubmitIndexContributor(c)
	return nil
}

// DeleteContributor soft-deletes a contributor and enqueues an index removal.
func (s *ContributorService) DeleteContributor(ctx context.Context, id string) error {
	if err := s.store.DeleteContributor(ctx, id); err != nil {
		return err
	}
	s.indexer.SubmitDeleteContributor(id)
	return nil
}

// GetBooksByContributor returns all books associated with a contributor.
func (s *ContributorService) GetBooksByContributor(ctx context.Context, contributorID string) ([]*domain.Book, error) {
	return s.store.GetBooksByContributor(ctx, contributorID)
}

// GetBookIDsByContributor returns the book IDs associated with a contributor.
func (s *ContributorService) GetBookIDsByContributor(ctx context.Context, contributorID string) ([]string, error) {
	return s.store.GetBookIDsByContributor(ctx, contributorID)
}

// GetAccessibleBookIDSet returns the set of book IDs the given user can access.
func (s *ContributorService) GetAccessibleBookIDSet(ctx context.Context, userID string) (map[string]bool, error) {
	return s.store.GetAccessibleBookIDSet(ctx, userID)
}

// MergeContributors merges the source contributor into the target contributor.
// The source is deleted from the index; the merged target is re-indexed.
func (s *ContributorService) MergeContributors(ctx context.Context, sourceID, targetID string) (*domain.Contributor, error) {
	c, err := s.store.MergeContributors(ctx, sourceID, targetID)
	if err != nil {
		return nil, err
	}
	s.indexer.SubmitDeleteContributor(sourceID)
	s.indexer.SubmitIndexContributor(c)
	return c, nil
}

// UnmergeContributor splits an alias back into a separate contributor.
// The resulting contributor (with the alias removed) is re-indexed.
func (s *ContributorService) UnmergeContributor(ctx context.Context, sourceID, aliasName string) (*domain.Contributor, error) {
	c, err := s.store.UnmergeContributor(ctx, sourceID, aliasName)
	if err != nil {
		return nil, err
	}
	s.indexer.SubmitIndexContributor(c)
	return c, nil
}
