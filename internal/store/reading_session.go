package store

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

// initSessions initializes the Sessions entity on the store.
// Indexes by user_book (for finding active session), book (for book stats), and user (for user history).
// Index keys include the session ID to ensure uniqueness (multiple sessions can exist for same user+book).
func (s *Store) initSessions() {
	s.Sessions = NewEntity[domain.BookReadingSession](s, "session:").
		WithIndex("user_book", func(session *domain.BookReadingSession) []string {
			return []string{session.UserID + ":" + session.BookID + ":" + session.ID}
		}).
		WithIndex("book", func(session *domain.BookReadingSession) []string {
			return []string{session.BookID + ":" + session.ID}
		}).
		WithIndex("user", func(session *domain.BookReadingSession) []string {
			return []string{session.UserID + ":" + session.ID}
		})
}

// GetActiveSession returns the active (unfinished) session for a user+book combination.
// Returns nil if no active session exists.
// Returns an error if multiple active sessions exist (should not happen).
func (s *Store) GetActiveSession(ctx context.Context, userID, bookID string) (*domain.BookReadingSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Query all sessions for this user+book by scanning the index prefix
	// Index format: "session:idx:user_book:userID:bookID:sessionID"
	// We want all entries starting with "userID:bookID:"
	allSessions, err := s.getAllSessionsWithPrefix(ctx, "user_book", userID+":"+bookID+":")
	if err != nil {
		return nil, fmt.Errorf("finding sessions for user %s book %s: %w", userID, bookID, err)
	}

	// Filter for active sessions (FinishedAt is nil)
	var activeSessions []*domain.BookReadingSession
	for _, session := range allSessions {
		if session.IsActive() {
			activeSessions = append(activeSessions, session)
		}
	}

	// Return nil if no active sessions
	if len(activeSessions) == 0 {
		return nil, nil
	}

	// Should only have one active session per user+book
	// If we have multiple, this is a data integrity issue - return the most recent
	if len(activeSessions) > 1 {
		if s.logger != nil {
			s.logger.Warn("multiple active sessions found for user+book",
				"user_id", userID,
				"book_id", bookID,
				"count", len(activeSessions))
		}

		// Return the most recently updated session
		mostRecent := activeSessions[0]
		for _, session := range activeSessions[1:] {
			if session.UpdatedAt.After(mostRecent.UpdatedAt) {
				mostRecent = session
			}
		}
		return mostRecent, nil
	}

	return activeSessions[0], nil
}

// getAllSessionsWithPrefix retrieves all sessions matching an index prefix.
// This is a helper for queries that need to scan multiple index entries.
func (s *Store) getAllSessionsWithPrefix(ctx context.Context, indexName, prefix string) ([]*domain.BookReadingSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	indexPrefix := []byte(s.Sessions.Prefix() + "idx:" + indexName + ":" + prefix)
	var sessions []*domain.BookReadingSession

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = indexPrefix
		opts.PrefetchValues = true

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(indexPrefix); it.ValidForPrefix(indexPrefix); it.Next() {
			// Get the session ID from the index value
			var sessionID string
			err := it.Item().Value(func(val []byte) error {
				sessionID = string(val)
				return nil
			})
			if err != nil {
				return err
			}

			// Get the actual session
			session, err := s.Sessions.Get(ctx, sessionID)
			if err != nil {
				// Skip if session not found (index cleanup issue)
				if errors.Is(err, ErrNotFound) {
					continue
				}
				return err
			}

			sessions = append(sessions, session)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return sessions, nil
}

// GetUserReadingSessions returns a user's reading sessions sorted by StartedAt descending.
// Limit controls how many sessions to return (0 = all).
func (s *Store) GetUserReadingSessions(ctx context.Context, userID string, limit int) ([]*domain.BookReadingSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Get all sessions for the user by scanning index prefix
	sessions, err := s.getAllSessionsWithPrefix(ctx, "user", userID+":")
	if err != nil {
		return nil, fmt.Errorf("finding sessions for user %s: %w", userID, err)
	}

	// Sort by StartedAt descending (most recent first)
	slices.SortFunc(sessions, func(a, b *domain.BookReadingSession) int {
		return b.StartedAt.Compare(a.StartedAt)
	})

	// Apply limit if specified
	if limit > 0 && len(sessions) > limit {
		sessions = sessions[:limit]
	}

	return sessions, nil
}

// ListAllSessions returns an iterator over all sessions.
// This is useful for cleanup operations or analytics.
func (s *Store) ListAllSessions(ctx context.Context) iter.Seq2[*domain.BookReadingSession, error] {
	return s.Sessions.List(ctx)
}

// GetReadingSession retrieves a reading session by ID.
// Returns ErrNotFound if the session does not exist.
func (s *Store) GetReadingSession(ctx context.Context, id string) (*domain.BookReadingSession, error) {
	session, err := s.Sessions.Get(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("session %s: %w", id, ErrNotFound)
		}
		return nil, fmt.Errorf("getting session %s: %w", id, err)
	}
	return session, nil
}

// CreateReadingSession creates a new reading session.
// Returns ErrAlreadyExists if a session with this ID already exists.
func (s *Store) CreateReadingSession(ctx context.Context, session *domain.BookReadingSession) error {
	if err := s.Sessions.Create(ctx, session.ID, session); err != nil {
		if errors.Is(err, ErrAlreadyExists) {
			return fmt.Errorf("session %s: %w", session.ID, ErrAlreadyExists)
		}
		return fmt.Errorf("creating session %s: %w", session.ID, err)
	}
	return nil
}

// UpdateReadingSession updates an existing reading session.
// Returns ErrNotFound if the session does not exist.
func (s *Store) UpdateReadingSession(ctx context.Context, session *domain.BookReadingSession) error {
	if err := s.Sessions.Update(ctx, session.ID, session); err != nil {
		if errors.Is(err, ErrNotFound) {
			return fmt.Errorf("session %s: %w", session.ID, ErrNotFound)
		}
		return fmt.Errorf("updating session %s: %w", session.ID, err)
	}
	return nil
}

// DeleteReadingSession deletes a reading session by ID.
// This operation is idempotent - it does not return an error if the session does not exist.
func (s *Store) DeleteReadingSession(ctx context.Context, id string) error {
	if err := s.Sessions.Delete(ctx, id); err != nil {
		return fmt.Errorf("deleting session %s: %w", id, err)
	}
	return nil
}

// GetBookSessions returns all sessions for a book (across all users).
// This is useful for book statistics (how many users started/completed this book).
func (s *Store) GetBookSessions(ctx context.Context, bookID string) ([]*domain.BookReadingSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	sessions, err := s.getAllSessionsWithPrefix(ctx, "book", bookID+":")
	if err != nil {
		return nil, fmt.Errorf("finding sessions for book %s: %w", bookID, err)
	}

	return sessions, nil
}

// GetUserBookSessions returns all sessions for a specific user and book combination.
// This is useful for checking if a user has previously read/completed a book.
func (s *Store) GetUserBookSessions(ctx context.Context, userID, bookID string) ([]*domain.BookReadingSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	sessions, err := s.getAllSessionsWithPrefix(ctx, "user_book", userID+":"+bookID+":")
	if err != nil {
		return nil, fmt.Errorf("finding sessions for user %s book %s: %w", userID, bookID, err)
	}

	return sessions, nil
}
