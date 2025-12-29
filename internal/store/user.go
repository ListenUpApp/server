package store

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/sse"
)

const (
	userPrefix           = "user:"
	userByEmailPrefix    = "idx:users:email:" // For login lookups
	sessionPrefix        = "session:"
	sessionByUserPrefix  = "idx:sessions:user:"  // For listing user sessions
	sessionByTokenPrefix = "idx:sessions:token:" // For refresh token lookups
)

var (
	// ErrUserNotFound is returned when a user cannot be found by ID or email.
	ErrUserNotFound = errors.New("user not found")
	// ErrUserExists is returned when attempting to create a user with an existing ID.
	ErrUserExists = errors.New("user already exists")
	// ErrEmailExists is returned when attempting to create a user with an email that's already in use.
	ErrEmailExists = errors.New("email already in use")
	// ErrSessionNotFound is returned when a session cannot be found by ID.
	ErrSessionNotFound = errors.New("session not found")
	// ErrSessionExpired is returned when attempting to use an expired session.
	ErrSessionExpired = errors.New("session expired")
)

// CreateUser creates a new user account.
func (s *Store) CreateUser(_ context.Context, user *domain.User) error {
	key := []byte(userPrefix + user.ID)

	// Checks if user ID already exists
	exists, err := s.exists(key)
	if err != nil {
		return fmt.Errorf("check user exists: %w", err)
	}

	if exists {
		return ErrUserExists
	}

	// Normalize email for index look
	normalizedEmail := normalizeEmail(user.Email)
	emailKey := []byte(userByEmailPrefix + normalizedEmail)

	return s.db.Update(func(txn *badger.Txn) error {
		// Check if email is already in use
		_, err := txn.Get(emailKey)
		if err == nil {
			return ErrEmailExists
		}
		if !errors.Is(err, badger.ErrKeyNotFound) {
			return fmt.Errorf("check email exists: %w", err)
		}

		// Save user
		data, err := json.Marshal(user)
		if err != nil {
			return fmt.Errorf("marshal user: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Create email index
		if err := txn.Set(emailKey, []byte(user.ID)); err != nil {
			return err
		}

		return nil
	})
}

// GetUser retrieves a user by ID.
func (s *Store) GetUser(_ context.Context, id string) (*domain.User, error) {
	key := []byte(userPrefix + id)

	var user domain.User
	if err := s.get(key, &user); err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user: %w", err)
	}

	// Check soft delete
	if user.IsDeleted() {
		return nil, ErrUserNotFound
	}

	return &user, nil
}

// GetUserByEmail retrieves a user by email address.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	normalizedEmail := normalizeEmail(email)
	emailKey := []byte(userByEmailPrefix + normalizedEmail)

	// Look up user ID from email index
	var userID string
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(emailKey)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			userID = string(val)
			return nil
		})
	})

	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("lookup user by email: %w", err)
	}

	// Get the actual user
	return s.GetUser(ctx, userID)
}

// UpdateUser updates an existing user.
func (s *Store) UpdateUser(ctx context.Context, user *domain.User) error {
	key := []byte(userPrefix + user.ID)

	// Get old user for email index updates
	oldUser, err := s.GetUser(ctx, user.ID)
	if err != nil {
		return err
	}

	user.Touch()

	return s.db.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(user)
		if err != nil {
			return fmt.Errorf("marshal user: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Update email index if email changed
		if oldUser.Email != user.Email {
			// Delete old email index
			oldEmailKey := []byte(userByEmailPrefix + normalizeEmail(oldUser.Email))
			if err := txn.Delete(oldEmailKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}

			// Check new email isn't in use
			newEmailKey := []byte(userByEmailPrefix + normalizeEmail(user.Email))
			_, err := txn.Get(newEmailKey)
			if err == nil {
				return ErrEmailExists
			}
			if !errors.Is(err, badger.ErrKeyNotFound) {
				return fmt.Errorf("check new email: %w", err)
			}

			// Create new email index
			if err := txn.Set(newEmailKey, []byte(user.ID)); err != nil {
				return err
			}
		}

		return nil
	})
}

// ListUsers returns all non-deleted users (for admin view).
func (s *Store) ListUsers(_ context.Context) ([]*domain.User, error) {
	prefix := []byte(userPrefix)
	var users []*domain.User

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var user domain.User
				if unmarshalErr := json.Unmarshal(val, &user); unmarshalErr != nil {
					// Skip malformed users
					return nil //nolint:nilerr // intentionally skip malformed entries
				}

				// Skip deleted users
				if user.IsDeleted() {
					return nil
				}

				users = append(users, &user)
				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	return users, nil
}

// ListPendingUsers returns all users with pending status (awaiting approval).
func (s *Store) ListPendingUsers(_ context.Context) ([]*domain.User, error) {
	prefix := []byte(userPrefix)
	var users []*domain.User

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(val []byte) error {
				var user domain.User
				if unmarshalErr := json.Unmarshal(val, &user); unmarshalErr != nil {
					// Skip malformed users
					return nil //nolint:nilerr // intentionally skip malformed entries
				}

				// Skip deleted users and non-pending users
				if user.IsDeleted() || !user.IsPending() {
					return nil
				}

				users = append(users, &user)
				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("list pending users: %w", err)
	}

	return users, nil
}

// normalizeEmail normalizes an email address for consistent lookups.
// Lowercases and trims whitespace.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// BroadcastUserPending broadcasts a user.pending SSE event to admin users.
// Called when a new user registers and is awaiting approval.
func (s *Store) BroadcastUserPending(user *domain.User) {
	if s.eventEmitter != nil {
		s.eventEmitter.Emit(sse.NewUserPendingEvent(user))
	}
}

// BroadcastUserApproved broadcasts a user.approved SSE event to admin users.
// Called when a pending user is approved by an admin.
func (s *Store) BroadcastUserApproved(user *domain.User) {
	if s.eventEmitter != nil {
		s.eventEmitter.Emit(sse.NewUserApprovedEvent(user))
	}
}
