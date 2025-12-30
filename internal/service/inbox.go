package service

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/dto"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
)

// InboxService manages the Inbox staging workflow.
// Books in Inbox are hidden from all library views until released.
type InboxService struct {
	store    *store.Store
	enricher *dto.Enricher
	sse      *sse.Manager
	logger   *slog.Logger
}

// NewInboxService creates a new inbox service.
func NewInboxService(store *store.Store, enricher *dto.Enricher, sseManager *sse.Manager, logger *slog.Logger) *InboxService {
	return &InboxService{
		store:    store,
		enricher: enricher,
		sse:      sseManager,
		logger:   logger,
	}
}

// ReleaseResult contains the result of a release operation.
type ReleaseResult struct {
	Released      int `json:"released"`
	Public        int `json:"public"`
	ToCollections int `json:"to_collections"`
}

// ListBooks returns all books in the Inbox.
func (s *InboxService) ListBooks(ctx context.Context) ([]*domain.Book, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	library, err := s.store.GetDefaultLibrary(ctx)
	if err != nil {
		return nil, fmt.Errorf("get default library: %w", err)
	}

	inbox, err := s.store.GetInboxForLibrary(ctx, library.ID)
	if err != nil {
		return nil, fmt.Errorf("get inbox: %w", err)
	}

	// Get all books in the inbox collection
	var books []*domain.Book
	for _, bookID := range inbox.BookIDs {
		book, err := s.store.GetBookNoAccessCheck(ctx, bookID)
		if err != nil {
			s.logger.Warn("failed to get inbox book",
				"book_id", bookID,
				"error", err,
			)
			continue
		}
		books = append(books, book)
	}

	return books, nil
}

// ReleaseBooks releases books from the Inbox to the library.
// Books are added to their staged collections (if any), then removed from Inbox.
func (s *InboxService) ReleaseBooks(ctx context.Context, bookIDs []string) (*ReleaseResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	library, err := s.store.GetDefaultLibrary(ctx)
	if err != nil {
		return nil, fmt.Errorf("get default library: %w", err)
	}

	inbox, err := s.store.GetInboxForLibrary(ctx, library.ID)
	if err != nil {
		return nil, fmt.Errorf("get inbox: %w", err)
	}

	result := &ReleaseResult{}

	for _, bookID := range bookIDs {
		book, err := s.store.GetBookNoAccessCheck(ctx, bookID)
		if err != nil {
			s.logger.Warn("failed to get book for release",
				"book_id", bookID,
				"error", err,
			)
			continue
		}

		// Track how many collections this book is going to
		stagedCount := len(book.StagedCollectionIDs)

		// Apply staged collections
		for _, collID := range book.StagedCollectionIDs {
			if err := s.store.AdminAddBookToCollection(ctx, bookID, collID); err != nil {
				s.logger.Warn("failed to add book to staged collection",
					"book_id", bookID,
					"collection_id", collID,
					"error", err,
				)
				continue
			}
			result.ToCollections++
		}

		if stagedCount == 0 {
			result.Public++
		}

		// Clear staged assignments
		book.StagedCollectionIDs = nil
		if err := s.store.UpdateBook(ctx, book); err != nil {
			s.logger.Error("failed to clear staged collections",
				"book_id", bookID,
				"error", err,
			)
			continue
		}

		// Remove from Inbox
		if err := s.store.AdminRemoveBookFromCollection(ctx, bookID, inbox.ID); err != nil {
			s.logger.Error("failed to remove book from inbox",
				"book_id", bookID,
				"error", err,
			)
			continue
		}

		// Emit book.created event (book now "exists" for users)
		bookDTO, err := s.enricher.EnrichBook(ctx, book)
		if err != nil {
			s.logger.Warn("failed to enrich book for SSE event",
				"book_id", bookID,
				"error", err,
			)
			// Still count as released, just won't emit proper SSE
		} else {
			s.sse.Emit(sse.NewBookCreatedEvent(bookDTO))
		}

		// Emit inbox.book_released event for admins
		s.sse.Emit(sse.NewInboxBookReleasedEvent(bookID))

		result.Released++

		s.logger.Info("book released from inbox",
			"book_id", bookID,
			"staged_collections", stagedCount,
		)
	}

	return result, nil
}

// StageCollection adds a collection to a book's staged assignments.
func (s *InboxService) StageCollection(ctx context.Context, bookID, collectionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	book, err := s.store.GetBookNoAccessCheck(ctx, bookID)
	if err != nil {
		return fmt.Errorf("get book: %w", err)
	}

	// Check book is in inbox
	if !s.isBookInInbox(ctx, bookID) {
		return fmt.Errorf("book %s is not in inbox", bookID)
	}

	// Check collection exists
	if _, err := s.store.AdminGetCollection(ctx, collectionID); err != nil {
		return fmt.Errorf("get collection: %w", err)
	}

	// Add to staged collections if not already present
	if !slices.Contains(book.StagedCollectionIDs, collectionID) {
		book.StagedCollectionIDs = append(book.StagedCollectionIDs, collectionID)
		if err := s.store.UpdateBook(ctx, book); err != nil {
			return fmt.Errorf("update book: %w", err)
		}
	}

	s.logger.Info("collection staged for book",
		"book_id", bookID,
		"collection_id", collectionID,
	)

	return nil
}

// UnstageCollection removes a collection from a book's staged assignments.
func (s *InboxService) UnstageCollection(ctx context.Context, bookID, collectionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	book, err := s.store.GetBookNoAccessCheck(ctx, bookID)
	if err != nil {
		return fmt.Errorf("get book: %w", err)
	}

	// Check book is in inbox
	if !s.isBookInInbox(ctx, bookID) {
		return fmt.Errorf("book %s is not in inbox", bookID)
	}

	// Remove from staged collections
	book.StagedCollectionIDs = slices.DeleteFunc(book.StagedCollectionIDs, func(id string) bool {
		return id == collectionID
	})

	if err := s.store.UpdateBook(ctx, book); err != nil {
		return fmt.Errorf("update book: %w", err)
	}

	s.logger.Info("collection unstaged for book",
		"book_id", bookID,
		"collection_id", collectionID,
	)

	return nil
}

// isBookInInbox checks if a book is in the Inbox collection.
func (s *InboxService) isBookInInbox(ctx context.Context, bookID string) bool {
	library, err := s.store.GetDefaultLibrary(ctx)
	if err != nil {
		return false
	}

	inbox, err := s.store.GetInboxForLibrary(ctx, library.ID)
	if err != nil {
		return false
	}

	return inbox.ContainsBook(bookID)
}

// GetInboxCount returns the number of books in the Inbox.
func (s *InboxService) GetInboxCount(ctx context.Context) (int, error) {
	library, err := s.store.GetDefaultLibrary(ctx)
	if err != nil {
		return 0, fmt.Errorf("get default library: %w", err)
	}

	inbox, err := s.store.GetInboxForLibrary(ctx, library.ID)
	if err != nil {
		return 0, fmt.Errorf("get inbox: %w", err)
	}

	return len(inbox.BookIDs), nil
}
