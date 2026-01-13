package store

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/listenupapp/listenup-server/internal/domain"
)

const (
	// Primary prefixes
	absImportPrefix        = "abs:import:"
	absImportUserPrefix    = "abs:import:user:"
	absImportBookPrefix    = "abs:import:book:"
	absImportSessionPrefix = "abs:import:session:"
	absImportProgressPrefix = "abs:import:progress:"

	// Index prefixes
	absImportByStatusPrefix       = "abs:idx:import:status:"
	absImportUserUnmappedPrefix   = "abs:idx:import:user:unmapped:"
	absImportBookUnmappedPrefix   = "abs:idx:import:book:unmapped:"
	absImportSessionByStatusPrefix = "abs:idx:import:session:status:"
)

var (
	ErrABSImportNotFound     = errors.New("ABS import not found")
	ErrABSImportExists       = errors.New("ABS import already exists")
	ErrABSImportUserNotFound = errors.New("ABS import user not found")
	ErrABSImportBookNotFound = errors.New("ABS import book not found")
	ErrABSImportSessionNotFound = errors.New("ABS import session not found")
)

// --- ABSImport CRUD ---

// CreateABSImport creates a new ABS import record.
func (s *Store) CreateABSImport(_ context.Context, imp *domain.ABSImport) error {
	key := s.absImportKey(imp.ID)

	exists, err := s.exists(key)
	if err != nil {
		return fmt.Errorf("check import exists: %w", err)
	}
	if exists {
		return ErrABSImportExists
	}

	return s.db.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(imp)
		if err != nil {
			return fmt.Errorf("marshal import: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Create status index
		statusKey := s.absImportStatusKey(imp.Status, imp.ID)
		if err := txn.Set(statusKey, []byte{}); err != nil {
			return err
		}

		return nil
	})
}

// GetABSImport retrieves an ABS import by ID.
func (s *Store) GetABSImport(_ context.Context, id string) (*domain.ABSImport, error) {
	key := s.absImportKey(id)

	var imp domain.ABSImport
	if err := s.get(key, &imp); err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrABSImportNotFound
		}
		return nil, fmt.Errorf("get import: %w", err)
	}

	return &imp, nil
}

// ListABSImports returns all ABS imports.
func (s *Store) ListABSImports(_ context.Context) ([]*domain.ABSImport, error) {
	prefix := []byte(absImportPrefix)
	var imports []*domain.ABSImport

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := item.Key()

			// Skip index keys and nested keys (users/books/sessions)
			if len(key) > len(prefix) {
				remainder := string(key[len(prefix):])
				// Skip if it contains another colon (nested key)
				if containsColon(remainder) {
					continue
				}
			}

			err := item.Value(func(val []byte) error {
				var imp domain.ABSImport
				if unmarshalErr := json.Unmarshal(val, &imp); unmarshalErr != nil {
					return nil //nolint:nilerr // skip malformed entries
				}
				imports = append(imports, &imp)
				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("list imports: %w", err)
	}

	return imports, nil
}

// UpdateABSImport updates an existing ABS import.
func (s *Store) UpdateABSImport(_ context.Context, imp *domain.ABSImport) error {
	key := s.absImportKey(imp.ID)

	// Get existing to check status change
	var existing domain.ABSImport
	if err := s.get(key, &existing); err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrABSImportNotFound
		}
		return fmt.Errorf("get existing import: %w", err)
	}

	imp.UpdatedAt = time.Now()

	return s.db.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(imp)
		if err != nil {
			return fmt.Errorf("marshal import: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Update status index if changed
		if existing.Status != imp.Status {
			// Delete old status index
			oldStatusKey := s.absImportStatusKey(existing.Status, imp.ID)
			if err := txn.Delete(oldStatusKey); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}

			// Create new status index
			newStatusKey := s.absImportStatusKey(imp.Status, imp.ID)
			if err := txn.Set(newStatusKey, []byte{}); err != nil {
				return err
			}
		}

		return nil
	})
}

// DeleteABSImport deletes an ABS import and all its associated data.
func (s *Store) DeleteABSImport(ctx context.Context, id string) error {
	imp, err := s.GetABSImport(ctx, id)
	if err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		// Delete main import
		if err := txn.Delete(s.absImportKey(id)); err != nil {
			return err
		}

		// Delete status index
		statusKey := s.absImportStatusKey(imp.Status, id)
		_ = txn.Delete(statusKey) // Ignore if not exists

		// Delete all users for this import
		if err := s.deletePrefixInTxn(txn, []byte(absImportUserPrefix+id+":")); err != nil {
			return err
		}

		// Delete all books for this import
		if err := s.deletePrefixInTxn(txn, []byte(absImportBookPrefix+id+":")); err != nil {
			return err
		}

		// Delete all sessions for this import
		if err := s.deletePrefixInTxn(txn, []byte(absImportSessionPrefix+id+":")); err != nil {
			return err
		}

		// Delete all progress entries for this import
		if err := s.deletePrefixInTxn(txn, []byte(absImportProgressPrefix+id+":")); err != nil {
			return err
		}

		// Delete unmapped indexes
		_ = s.deletePrefixInTxn(txn, []byte(absImportUserUnmappedPrefix+id+":"))
		_ = s.deletePrefixInTxn(txn, []byte(absImportBookUnmappedPrefix+id+":"))
		_ = s.deletePrefixInTxn(txn, []byte(absImportSessionByStatusPrefix+id+":"))

		return nil
	})
}

// --- ABSImportUser CRUD ---

// CreateABSImportUser creates a new ABS import user.
func (s *Store) CreateABSImportUser(_ context.Context, user *domain.ABSImportUser) error {
	key := s.absImportUserKey(user.ImportID, user.ABSUserID)

	return s.db.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(user)
		if err != nil {
			return fmt.Errorf("marshal import user: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Create unmapped index if not mapped
		if !user.IsMapped() {
			unmappedKey := s.absImportUserUnmappedKey(user.ImportID, user.ABSUserID)
			if err := txn.Set(unmappedKey, []byte{}); err != nil {
				return err
			}
		}

		return nil
	})
}

// GetABSImportUser retrieves an ABS import user.
func (s *Store) GetABSImportUser(_ context.Context, importID, absUserID string) (*domain.ABSImportUser, error) {
	key := s.absImportUserKey(importID, absUserID)

	var user domain.ABSImportUser
	if err := s.get(key, &user); err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrABSImportUserNotFound
		}
		return nil, fmt.Errorf("get import user: %w", err)
	}

	return &user, nil
}

// ListABSImportUsers returns ABS import users with optional filter.
func (s *Store) ListABSImportUsers(_ context.Context, importID string, filter domain.MappingFilter) ([]*domain.ABSImportUser, error) {
	prefix := []byte(absImportUserPrefix + importID + ":")
	var users []*domain.ABSImportUser

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			err := it.Item().Value(func(val []byte) error {
				var user domain.ABSImportUser
				if unmarshalErr := json.Unmarshal(val, &user); unmarshalErr != nil {
					return nil //nolint:nilerr
				}

				// Apply filter
				switch filter {
				case domain.MappingFilterMapped:
					if !user.IsMapped() {
						return nil
					}
				case domain.MappingFilterUnmapped:
					if user.IsMapped() {
						return nil
					}
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
		return nil, fmt.Errorf("list import users: %w", err)
	}

	return users, nil
}

// UpdateABSImportUserMapping updates the mapping for an ABS import user.
func (s *Store) UpdateABSImportUserMapping(ctx context.Context, importID, absUserID string, listenUpID *string) error {
	user, err := s.GetABSImportUser(ctx, importID, absUserID)
	if err != nil {
		return err
	}

	wasMapped := user.IsMapped()
	user.ListenUpID = listenUpID
	if listenUpID != nil {
		now := time.Now()
		user.MappedAt = &now
	} else {
		user.MappedAt = nil
	}
	isMapped := user.IsMapped()

	key := s.absImportUserKey(importID, absUserID)
	unmappedKey := s.absImportUserUnmappedKey(importID, absUserID)

	return s.db.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(user)
		if err != nil {
			return fmt.Errorf("marshal import user: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Update unmapped index
		if !wasMapped && isMapped {
			// Was unmapped, now mapped - delete from unmapped index
			_ = txn.Delete(unmappedKey)
		} else if wasMapped && !isMapped {
			// Was mapped, now unmapped - add to unmapped index
			if err := txn.Set(unmappedKey, []byte{}); err != nil {
				return err
			}
		}

		return nil
	})
}

// --- ABSImportBook CRUD ---

// CreateABSImportBook creates a new ABS import book.
func (s *Store) CreateABSImportBook(_ context.Context, book *domain.ABSImportBook) error {
	key := s.absImportBookKey(book.ImportID, book.ABSMediaID)

	return s.db.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(book)
		if err != nil {
			return fmt.Errorf("marshal import book: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Create unmapped index if not mapped
		if !book.IsMapped() {
			unmappedKey := s.absImportBookUnmappedKey(book.ImportID, book.ABSMediaID)
			if err := txn.Set(unmappedKey, []byte{}); err != nil {
				return err
			}
		}

		return nil
	})
}

// GetABSImportBook retrieves an ABS import book.
func (s *Store) GetABSImportBook(_ context.Context, importID, absMediaID string) (*domain.ABSImportBook, error) {
	key := s.absImportBookKey(importID, absMediaID)

	var book domain.ABSImportBook
	if err := s.get(key, &book); err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrABSImportBookNotFound
		}
		return nil, fmt.Errorf("get import book: %w", err)
	}

	return &book, nil
}

// ListABSImportBooks returns ABS import books with optional filter.
func (s *Store) ListABSImportBooks(_ context.Context, importID string, filter domain.MappingFilter) ([]*domain.ABSImportBook, error) {
	prefix := []byte(absImportBookPrefix + importID + ":")
	var books []*domain.ABSImportBook

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			err := it.Item().Value(func(val []byte) error {
				var book domain.ABSImportBook
				if unmarshalErr := json.Unmarshal(val, &book); unmarshalErr != nil {
					return nil //nolint:nilerr
				}

				// Apply filter
				switch filter {
				case domain.MappingFilterMapped:
					if !book.IsMapped() {
						return nil
					}
				case domain.MappingFilterUnmapped:
					if book.IsMapped() {
						return nil
					}
				}

				books = append(books, &book)
				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("list import books: %w", err)
	}

	return books, nil
}

// UpdateABSImportBookMapping updates the mapping for an ABS import book.
func (s *Store) UpdateABSImportBookMapping(ctx context.Context, importID, absMediaID string, listenUpID *string) error {
	book, err := s.GetABSImportBook(ctx, importID, absMediaID)
	if err != nil {
		return err
	}

	wasMapped := book.IsMapped()
	book.ListenUpID = listenUpID
	if listenUpID != nil {
		now := time.Now()
		book.MappedAt = &now
	} else {
		book.MappedAt = nil
	}
	isMapped := book.IsMapped()

	key := s.absImportBookKey(importID, absMediaID)
	unmappedKey := s.absImportBookUnmappedKey(importID, absMediaID)

	return s.db.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(book)
		if err != nil {
			return fmt.Errorf("marshal import book: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Update unmapped index
		if !wasMapped && isMapped {
			_ = txn.Delete(unmappedKey)
		} else if wasMapped && !isMapped {
			if err := txn.Set(unmappedKey, []byte{}); err != nil {
				return err
			}
		}

		return nil
	})
}

// --- ABSImportSession CRUD ---

// CreateABSImportSession creates a new ABS import session.
func (s *Store) CreateABSImportSession(_ context.Context, session *domain.ABSImportSession) error {
	key := s.absImportSessionKey(session.ImportID, session.ABSSessionID)

	return s.db.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(session)
		if err != nil {
			return fmt.Errorf("marshal import session: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Create status index
		statusKey := s.absImportSessionStatusKey(session.ImportID, session.Status, session.ABSSessionID)
		if err := txn.Set(statusKey, []byte{}); err != nil {
			return err
		}

		return nil
	})
}

// GetABSImportSession retrieves an ABS import session.
func (s *Store) GetABSImportSession(_ context.Context, importID, sessionID string) (*domain.ABSImportSession, error) {
	key := s.absImportSessionKey(importID, sessionID)

	var session domain.ABSImportSession
	if err := s.get(key, &session); err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, ErrABSImportSessionNotFound
		}
		return nil, fmt.Errorf("get import session: %w", err)
	}

	return &session, nil
}

// ListABSImportSessions returns ABS import sessions with optional status filter.
func (s *Store) ListABSImportSessions(_ context.Context, importID string, filter domain.SessionStatusFilter) ([]*domain.ABSImportSession, error) {
	prefix := []byte(absImportSessionPrefix + importID + ":")
	var sessions []*domain.ABSImportSession

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			err := it.Item().Value(func(val []byte) error {
				var session domain.ABSImportSession
				if unmarshalErr := json.Unmarshal(val, &session); unmarshalErr != nil {
					return nil //nolint:nilerr
				}

				// Apply filter
				switch filter {
				case domain.SessionFilterPending:
					if session.Status != domain.SessionStatusPendingUser && session.Status != domain.SessionStatusPendingBook {
						return nil
					}
				case domain.SessionFilterReady:
					if session.Status != domain.SessionStatusReady {
						return nil
					}
				case domain.SessionFilterImported:
					if session.Status != domain.SessionStatusImported {
						return nil
					}
				case domain.SessionFilterSkipped:
					if session.Status != domain.SessionStatusSkipped {
						return nil
					}
				}

				sessions = append(sessions, &session)
				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("list import sessions: %w", err)
	}

	return sessions, nil
}

// UpdateABSImportSessionStatus updates the status of an ABS import session.
func (s *Store) UpdateABSImportSessionStatus(ctx context.Context, importID, sessionID string, status domain.SessionImportStatus) error {
	session, err := s.GetABSImportSession(ctx, importID, sessionID)
	if err != nil {
		return err
	}

	oldStatus := session.Status
	session.Status = status

	if status == domain.SessionStatusImported {
		now := time.Now()
		session.ImportedAt = &now
	}

	key := s.absImportSessionKey(importID, sessionID)

	return s.db.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(session)
		if err != nil {
			return fmt.Errorf("marshal import session: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Update status index
		oldStatusKey := s.absImportSessionStatusKey(importID, oldStatus, sessionID)
		_ = txn.Delete(oldStatusKey)

		newStatusKey := s.absImportSessionStatusKey(importID, status, sessionID)
		if err := txn.Set(newStatusKey, []byte{}); err != nil {
			return err
		}

		return nil
	})
}

// SkipABSImportSession marks a session as skipped with a reason.
func (s *Store) SkipABSImportSession(ctx context.Context, importID, sessionID, reason string) error {
	session, err := s.GetABSImportSession(ctx, importID, sessionID)
	if err != nil {
		return err
	}

	oldStatus := session.Status
	session.Status = domain.SessionStatusSkipped
	session.SkipReason = &reason

	key := s.absImportSessionKey(importID, sessionID)

	return s.db.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(session)
		if err != nil {
			return fmt.Errorf("marshal import session: %w", err)
		}

		if err := txn.Set(key, data); err != nil {
			return err
		}

		// Update status index
		oldStatusKey := s.absImportSessionStatusKey(importID, oldStatus, sessionID)
		_ = txn.Delete(oldStatusKey)

		newStatusKey := s.absImportSessionStatusKey(importID, domain.SessionStatusSkipped, sessionID)
		if err := txn.Set(newStatusKey, []byte{}); err != nil {
			return err
		}

		return nil
	})
}

// RecalculateSessionStatuses updates all session statuses based on current mappings.
// Call this after mapping changes to update which sessions are ready.
func (s *Store) RecalculateSessionStatuses(ctx context.Context, importID string) error {
	// Get all users and books to build mapping lookup
	users, err := s.ListABSImportUsers(ctx, importID, domain.MappingFilterAll)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}

	books, err := s.ListABSImportBooks(ctx, importID, domain.MappingFilterAll)
	if err != nil {
		return fmt.Errorf("list books: %w", err)
	}

	// Build lookup maps
	userMapped := make(map[string]bool)
	for _, u := range users {
		userMapped[u.ABSUserID] = u.IsMapped()
	}

	bookMapped := make(map[string]bool)
	for _, b := range books {
		bookMapped[b.ABSMediaID] = b.IsMapped()
	}

	// Get all sessions (excluding already imported/skipped)
	sessions, err := s.ListABSImportSessions(ctx, importID, domain.SessionFilterAll)
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	// Update each session's status
	for _, session := range sessions {
		// Skip already final states
		if session.Status == domain.SessionStatusImported || session.Status == domain.SessionStatusSkipped {
			continue
		}

		userOK := userMapped[session.ABSUserID]
		bookOK := bookMapped[session.ABSMediaID]

		var newStatus domain.SessionImportStatus
		switch {
		case userOK && bookOK:
			newStatus = domain.SessionStatusReady
		case !userOK:
			newStatus = domain.SessionStatusPendingUser
		default:
			newStatus = domain.SessionStatusPendingBook
		}

		if session.Status != newStatus {
			if err := s.UpdateABSImportSessionStatus(ctx, importID, session.ABSSessionID, newStatus); err != nil {
				return fmt.Errorf("update session %s: %w", session.ABSSessionID, err)
			}
		}
	}

	return nil
}

// GetABSImportStats recalculates and returns current stats for an import.
func (s *Store) GetABSImportStats(ctx context.Context, importID string) (mapped, unmapped, ready, imported int, err error) {
	users, err := s.ListABSImportUsers(ctx, importID, domain.MappingFilterAll)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	books, err := s.ListABSImportBooks(ctx, importID, domain.MappingFilterAll)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	sessions, err := s.ListABSImportSessions(ctx, importID, domain.SessionFilterAll)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	usersMapped := 0
	for _, u := range users {
		if u.IsMapped() {
			usersMapped++
		}
	}

	booksMapped := 0
	for _, b := range books {
		if b.IsMapped() {
			booksMapped++
		}
	}

	sessionsReady := 0
	sessionsImported := 0
	for _, s := range sessions {
		switch s.Status {
		case domain.SessionStatusReady:
			sessionsReady++
		case domain.SessionStatusImported:
			sessionsImported++
		}
	}

	return usersMapped + booksMapped, (len(users) - usersMapped) + (len(books) - booksMapped), sessionsReady, sessionsImported, nil
}

// --- Key builders ---

func (s *Store) absImportKey(id string) []byte {
	return []byte(absImportPrefix + id)
}

func (s *Store) absImportStatusKey(status domain.ABSImportStatus, id string) []byte {
	return []byte(absImportByStatusPrefix + string(status) + ":" + id)
}

func (s *Store) absImportUserKey(importID, absUserID string) []byte {
	return []byte(absImportUserPrefix + importID + ":" + absUserID)
}

func (s *Store) absImportUserUnmappedKey(importID, absUserID string) []byte {
	return []byte(absImportUserUnmappedPrefix + importID + ":" + absUserID)
}

func (s *Store) absImportBookKey(importID, absMediaID string) []byte {
	return []byte(absImportBookPrefix + importID + ":" + absMediaID)
}

func (s *Store) absImportBookUnmappedKey(importID, absMediaID string) []byte {
	return []byte(absImportBookUnmappedPrefix + importID + ":" + absMediaID)
}

func (s *Store) absImportSessionKey(importID, sessionID string) []byte {
	return []byte(absImportSessionPrefix + importID + ":" + sessionID)
}

func (s *Store) absImportSessionStatusKey(importID string, status domain.SessionImportStatus, sessionID string) []byte {
	return []byte(absImportSessionByStatusPrefix + importID + ":" + string(status) + ":" + sessionID)
}

// --- Helper functions ---

func containsColon(s string) bool {
	for i := range len(s) {
		if s[i] == ':' {
			return true
		}
	}
	return false
}

func (s *Store) deletePrefixInTxn(txn *badger.Txn, prefix []byte) error {
	opts := badger.DefaultIteratorOptions
	opts.Prefix = prefix
	opts.PrefetchValues = false // Only need keys

	it := txn.NewIterator(opts)
	defer it.Close()

	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		key := it.Item().KeyCopy(nil)
		if err := txn.Delete(key); err != nil {
			return err
		}
	}

	return nil
}
