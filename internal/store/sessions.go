package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

// CreateSession creates a new user session.
func (s *Store) CreateSession(ctx context.Context, session *domain.Session) error {
	key := []byte(sessionPrefix + session.ID)

	exists, err := s.exists(key)
	if err != nil {
		return fmt.Errorf("check session exists: %w", err)
	}
	if exists {
		return fmt.Errorf("session already exists")
	}

	tokenKey := []byte(sessionByTokenPrefix + session.RefreshTokenHash)
	userIndexKey := []byte(sessionByUserPrefix + session.UserID + ":" + session.ID)

	return s.db.Update(func(txn *badger.Txn) error {
		// Save session
		data, err := json.Marshal(session)
		if err != nil {
			return fmt.Errorf("marshal session: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Create refresh token index
		if err := txn.Set(tokenKey, []byte(session.ID)); err != nil {
			return err
		}

		// Create user index for listing sessions
		if err := txn.Set(userIndexKey, []byte{}); err != nil {
			return err
		}

		return nil
	})
}

// GetSession retrieves a session by ID.
func (s *Store) GetSession(_ context.Context, id string) (*domain.Session, error) {
	key := []byte(sessionPrefix + id)

	var session domain.Session
	if err := s.get(key, &session); err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("get session: %w", err)
	}

	// Check expiration
	if session.IsExpired() {
		return nil, ErrSessionExpired
	}

	return &session, nil
}

// GetSessionByRefreshToken retrieves a session by its refresh token hash.
// This is used during token refresh flow.
func (s *Store) GetSessionByRefreshToken(ctx context.Context, tokenHash string) (*domain.Session, error) {
	tokenKey := []byte(sessionByTokenPrefix + tokenHash)

	// Look up session ID from token index
	var sessionID string
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(tokenKey)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			sessionID = string(val)
			return nil
		})
	})

	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("lookup session by token: %w", err)
	}

	return s.GetSession(ctx, sessionID)
}

// UpdateSession updates an existing session (used for token rotation and last seen).
func (s *Store) UpdateSession(ctx context.Context, session *domain.Session) error {
	key := []byte(sessionPrefix + session.ID)

	// Get old session for token index updates
	oldSession, err := s.GetSession(ctx, session.ID)
	if err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(session)
		if err != nil {
			return fmt.Errorf("marshal session: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Update token index if token changed (rotation)
		if oldSession.RefreshTokenHash != session.RefreshTokenHash {
			// Delete old token index
			oldTokenKey := []byte(sessionByTokenPrefix + oldSession.RefreshTokenHash)
			if err := txn.Delete(oldTokenKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}

			// Create new token index
			newTokenKey := []byte(sessionByTokenPrefix + session.RefreshTokenHash)
			if err := txn.Set(newTokenKey, []byte(session.ID)); err != nil {
				return err
			}
		}

		return nil
	})
}

// DeleteSession deletes a session (logout).
func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	key := []byte(sessionPrefix + sessionID)

	// Get session data (even if expired) to clean up indices
	var session domain.Session
	if err := s.get(key, &session); err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil // Already gone
		}
		return fmt.Errorf("get session for deletion: %w", err)
	}

	tokenKey := []byte(sessionByTokenPrefix + session.RefreshTokenHash)
	userIndexKey := []byte(sessionByUserPrefix + session.UserID + ":" + sessionID)

	return s.db.Update(func(txn *badger.Txn) error {
		// Delete session
		if err := txn.Delete(key); err != nil {
			return err
		}

		// Delete token index
		if err := txn.Delete(tokenKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		// Delete user index
		if err := txn.Delete(userIndexKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		return nil
	})
}

// ListUserSessions returns all active sessions for a user.
func (s *Store) ListUserSessions(ctx context.Context, userID string) ([]*domain.Session, error) {
	prefix := []byte(sessionByUserPrefix + userID + ":")
	var sessions []*domain.Session

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		opts.PrefetchValues = false // We only need keys

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			// Extract session ID from key
			// Key format: idx:sessions:user:userID:sessionID
			key := string(it.Item().Key())
			parts := strings.Split(key, ":")
			if len(parts) < 5 {
				continue
			}
			sessionID := parts[4]

			// Get full session
			session, err := s.GetSession(ctx, sessionID)
			if err != nil {
				if errors.Is(err, ErrSessionExpired) || errors.Is(err, ErrSessionNotFound) {
					continue // Skip expired/missing sessions
				}
				return err
			}

			sessions = append(sessions, session)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("list user sessions: %w", err)
	}

	return sessions, nil
}

// DeleteExpiredSessions removes all expired sessions (cleanup job).
// This should be run periodically, like maybe on a daily basis.
// Just want to figure out how I want to do that...
func (s *Store) DeleteExpiredSessions(ctx context.Context) (int, error) {
	prefix := []byte(sessionPrefix)
	var expiredIDs []string

	// First pass: find expired sessions
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var session domain.Session
				if err := json.Unmarshal(val, &session); err != nil {
					return nil // Skip malformed sessions
				}

				if session.IsExpired() {
					expiredIDs = append(expiredIDs, session.ID)
				}

				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return 0, fmt.Errorf("find expired sessions: %w", err)
	}

	// Second pass: delete expired sessions
	for _, sessionID := range expiredIDs {
		if err := s.DeleteSession(ctx, sessionID); err != nil {
			s.logger.Warn("failed to delete expired session", "session_id", sessionID, "error", err)
		}
	}

	return len(expiredIDs), nil
}
