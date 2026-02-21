package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// insertTestUser inserts a minimal user row to satisfy foreign-key constraints.
func insertTestUser(t *testing.T, s *Store, userID string) {
	t.Helper()
	ctx := context.Background()
	user := makeTestUser(userID, userID+"@example.com")
	if err := s.CreateUser(ctx, user); err != nil {
		if !errors.Is(err, store.ErrAlreadyExists) {
			t.Fatalf("insertTestUser(%s): %v", userID, err)
		}
	}
}

// insertTestLibrary inserts a minimal library row to satisfy foreign-key constraints.
// The ownerID must already exist in the users table.
func insertTestLibrary(t *testing.T, s *Store, libID, ownerID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(`INSERT INTO libraries (id, created_at, updated_at, owner_id, name) VALUES (?,?,?,?,?)`,
		libID, now, now, ownerID, "Test Library")
	if err != nil {
		t.Fatalf("insert test library: %v", err)
	}
}

func TestCreateAndGetCollection(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Set up FK dependencies.
	insertTestUser(t, s, "user-1")
	insertTestLibrary(t, s, "lib-1", "user-1")
	insertTestBook(t, s, "book-1", "Book One", "/books/one")
	insertTestBook(t, s, "book-2", "Book Two", "/books/two")

	now := time.Now()
	coll := &domain.Collection{
		CreatedAt:      now,
		UpdatedAt:      now,
		ID:             "coll-1",
		LibraryID:      "lib-1",
		OwnerID:        "user-1",
		Name:           "My Favorites",
		BookIDs:        []string{"book-1", "book-2"},
		IsInbox:        true,
		IsGlobalAccess: true,
	}

	if err := s.CreateCollection(ctx, coll); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	got, err := s.GetCollection(ctx, "coll-1")
	if err != nil {
		t.Fatalf("GetCollection: %v", err)
	}

	if got.ID != coll.ID {
		t.Errorf("ID: got %q, want %q", got.ID, coll.ID)
	}
	if got.LibraryID != coll.LibraryID {
		t.Errorf("LibraryID: got %q, want %q", got.LibraryID, coll.LibraryID)
	}
	if got.OwnerID != coll.OwnerID {
		t.Errorf("OwnerID: got %q, want %q", got.OwnerID, coll.OwnerID)
	}
	if got.Name != coll.Name {
		t.Errorf("Name: got %q, want %q", got.Name, coll.Name)
	}
	if !got.IsInbox {
		t.Error("IsInbox: expected true")
	}
	if !got.IsGlobalAccess {
		t.Error("IsGlobalAccess: expected true")
	}

	// Timestamps should round-trip.
	if got.CreatedAt.Unix() != coll.CreatedAt.Unix() {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, coll.CreatedAt)
	}
	if got.UpdatedAt.Unix() != coll.UpdatedAt.Unix() {
		t.Errorf("UpdatedAt: got %v, want %v", got.UpdatedAt, coll.UpdatedAt)
	}

	// Verify BookIDs.
	if len(got.BookIDs) != 2 {
		t.Fatalf("BookIDs: got %d, want 2", len(got.BookIDs))
	}
	// BookIDs order may vary; check both are present.
	bookSet := map[string]bool{}
	for _, id := range got.BookIDs {
		bookSet[id] = true
	}
	if !bookSet["book-1"] {
		t.Error("BookIDs: missing book-1")
	}
	if !bookSet["book-2"] {
		t.Error("BookIDs: missing book-2")
	}
}

func TestGetCollection_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetCollection(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var storeErr *store.Error
	if !errors.As(err, &storeErr) {
		t.Fatalf("expected *store.Error, got %T: %v", err, err)
	}
	if storeErr.Code != store.ErrNotFound.Code {
		t.Errorf("expected status %d, got %d", store.ErrNotFound.Code, storeErr.Code)
	}
}

func TestUpdateCollection(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "user-1")
	insertTestLibrary(t, s, "lib-1", "user-1")
	insertTestBook(t, s, "book-1", "Book One", "/books/one")
	insertTestBook(t, s, "book-2", "Book Two", "/books/two")
	insertTestBook(t, s, "book-3", "Book Three", "/books/three")

	now := time.Now()
	coll := &domain.Collection{
		CreatedAt: now,
		UpdatedAt: now,
		ID:        "coll-upd",
		LibraryID: "lib-1",
		OwnerID:   "user-1",
		Name:      "Original",
		BookIDs:   []string{"book-1"},
	}

	if err := s.CreateCollection(ctx, coll); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	// Modify name and BookIDs.
	coll.Name = "Updated Name"
	coll.BookIDs = []string{"book-2", "book-3"}
	coll.UpdatedAt = time.Now()

	if err := s.UpdateCollection(ctx, coll); err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}

	got, err := s.GetCollection(ctx, "coll-upd")
	if err != nil {
		t.Fatalf("GetCollection after update: %v", err)
	}

	if got.Name != "Updated Name" {
		t.Errorf("Name: got %q, want %q", got.Name, "Updated Name")
	}
	if len(got.BookIDs) != 2 {
		t.Fatalf("BookIDs: got %d, want 2", len(got.BookIDs))
	}

	bookSet := map[string]bool{}
	for _, id := range got.BookIDs {
		bookSet[id] = true
	}
	if bookSet["book-1"] {
		t.Error("BookIDs: book-1 should have been removed")
	}
	if !bookSet["book-2"] {
		t.Error("BookIDs: missing book-2")
	}
	if !bookSet["book-3"] {
		t.Error("BookIDs: missing book-3")
	}
}

func TestDeleteCollection(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "user-1")
	insertTestLibrary(t, s, "lib-1", "user-1")

	now := time.Now()
	coll := &domain.Collection{
		CreatedAt: now,
		UpdatedAt: now,
		ID:        "coll-del",
		LibraryID: "lib-1",
		OwnerID:   "user-1",
		Name:      "Delete Me",
	}

	if err := s.CreateCollection(ctx, coll); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	// Verify it exists.
	_, err := s.GetCollection(ctx, "coll-del")
	if err != nil {
		t.Fatalf("GetCollection before delete: %v", err)
	}

	// Hard delete.
	if err := s.DeleteCollection(ctx, "coll-del"); err != nil {
		t.Fatalf("DeleteCollection: %v", err)
	}

	// Should be gone.
	_, err = s.GetCollection(ctx, "coll-del")
	if err == nil {
		t.Fatal("expected not found after delete, got nil")
	}
	var storeErr *store.Error
	if !errors.As(err, &storeErr) {
		t.Fatalf("expected *store.Error, got %T: %v", err, err)
	}
	if storeErr.Code != store.ErrNotFound.Code {
		t.Errorf("expected status %d, got %d", store.ErrNotFound.Code, storeErr.Code)
	}
}

func TestListCollectionsByLibrary(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "user-1")
	insertTestLibrary(t, s, "lib-1", "user-1")

	now := time.Now()
	coll1 := &domain.Collection{
		CreatedAt: now,
		UpdatedAt: now,
		ID:        "coll-list-1",
		LibraryID: "lib-1",
		OwnerID:   "user-1",
		Name:      "First",
	}
	coll2 := &domain.Collection{
		CreatedAt: now.Add(1 * time.Second),
		UpdatedAt: now.Add(1 * time.Second),
		ID:        "coll-list-2",
		LibraryID: "lib-1",
		OwnerID:   "user-1",
		Name:      "Second",
	}

	if err := s.CreateCollection(ctx, coll1); err != nil {
		t.Fatalf("CreateCollection 1: %v", err)
	}
	if err := s.CreateCollection(ctx, coll2); err != nil {
		t.Fatalf("CreateCollection 2: %v", err)
	}

	collections, err := s.ListCollectionsByLibrary(ctx, "lib-1")
	if err != nil {
		t.Fatalf("ListCollectionsByLibrary: %v", err)
	}

	if len(collections) != 2 {
		t.Fatalf("got %d collections, want 2", len(collections))
	}
	if collections[0].ID != "coll-list-1" {
		t.Errorf("collections[0].ID: got %q, want %q", collections[0].ID, "coll-list-1")
	}
	if collections[1].ID != "coll-list-2" {
		t.Errorf("collections[1].ID: got %q, want %q", collections[1].ID, "coll-list-2")
	}
}

func TestAddAndRemoveBookFromCollection(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "user-1")
	insertTestLibrary(t, s, "lib-1", "user-1")
	insertTestBook(t, s, "book-1", "Book One", "/books/one")

	now := time.Now()
	coll := &domain.Collection{
		CreatedAt: now,
		UpdatedAt: now,
		ID:        "coll-ab",
		LibraryID: "lib-1",
		OwnerID:   "user-1",
		Name:      "Add/Remove Test",
	}

	if err := s.CreateCollection(ctx, coll); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	// Add a book.
	if err := s.AddBookToCollection(ctx, "coll-ab", "book-1"); err != nil {
		t.Fatalf("AddBookToCollection: %v", err)
	}

	got, err := s.GetCollection(ctx, "coll-ab")
	if err != nil {
		t.Fatalf("GetCollection after add: %v", err)
	}
	if len(got.BookIDs) != 1 {
		t.Fatalf("BookIDs after add: got %d, want 1", len(got.BookIDs))
	}
	if got.BookIDs[0] != "book-1" {
		t.Errorf("BookIDs[0]: got %q, want %q", got.BookIDs[0], "book-1")
	}

	// Remove the book.
	if err := s.RemoveBookFromCollection(ctx, "coll-ab", "book-1"); err != nil {
		t.Fatalf("RemoveBookFromCollection: %v", err)
	}

	got, err = s.GetCollection(ctx, "coll-ab")
	if err != nil {
		t.Fatalf("GetCollection after remove: %v", err)
	}
	if len(got.BookIDs) != 0 {
		t.Errorf("BookIDs after remove: got %d, want 0", len(got.BookIDs))
	}
}

func TestGetCollectionsForBook(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "user-1")
	insertTestLibrary(t, s, "lib-1", "user-1")
	insertTestBook(t, s, "book-1", "Book One", "/books/one")

	now := time.Now()
	coll1 := &domain.Collection{
		CreatedAt: now,
		UpdatedAt: now,
		ID:        "coll-fb-1",
		LibraryID: "lib-1",
		OwnerID:   "user-1",
		Name:      "Collection A",
		BookIDs:   []string{"book-1"},
	}
	coll2 := &domain.Collection{
		CreatedAt: now,
		UpdatedAt: now,
		ID:        "coll-fb-2",
		LibraryID: "lib-1",
		OwnerID:   "user-1",
		Name:      "Collection B",
		BookIDs:   []string{"book-1"},
	}

	if err := s.CreateCollection(ctx, coll1); err != nil {
		t.Fatalf("CreateCollection 1: %v", err)
	}
	if err := s.CreateCollection(ctx, coll2); err != nil {
		t.Fatalf("CreateCollection 2: %v", err)
	}

	collections, err := s.GetCollectionsForBook(ctx, "book-1")
	if err != nil {
		t.Fatalf("GetCollectionsForBook: %v", err)
	}

	if len(collections) != 2 {
		t.Fatalf("got %d collections, want 2", len(collections))
	}

	idSet := map[string]bool{}
	for _, c := range collections {
		idSet[c.ID] = true
	}
	if !idSet["coll-fb-1"] {
		t.Error("missing coll-fb-1")
	}
	if !idSet["coll-fb-2"] {
		t.Error("missing coll-fb-2")
	}
}

func TestCreateAndGetShare(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "owner-1")
	insertTestUser(t, s, "friend-1")
	insertTestLibrary(t, s, "lib-1", "owner-1")

	now := time.Now()
	coll := &domain.Collection{
		CreatedAt: now,
		UpdatedAt: now,
		ID:        "coll-share",
		LibraryID: "lib-1",
		OwnerID:   "owner-1",
		Name:      "Shared Collection",
	}
	if err := s.CreateCollection(ctx, coll); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	share := &domain.CollectionShare{
		Syncable: domain.Syncable{
			ID:        "share-1",
			CreatedAt: now,
			UpdatedAt: now,
		},
		CollectionID:     "coll-share",
		SharedWithUserID: "friend-1",
		SharedByUserID:   "owner-1",
		Permission:       domain.PermissionWrite,
	}

	if err := s.CreateShare(ctx, share); err != nil {
		t.Fatalf("CreateShare: %v", err)
	}

	got, err := s.GetShare(ctx, "share-1")
	if err != nil {
		t.Fatalf("GetShare: %v", err)
	}

	if got.ID != share.ID {
		t.Errorf("ID: got %q, want %q", got.ID, share.ID)
	}
	if got.CollectionID != share.CollectionID {
		t.Errorf("CollectionID: got %q, want %q", got.CollectionID, share.CollectionID)
	}
	if got.SharedWithUserID != share.SharedWithUserID {
		t.Errorf("SharedWithUserID: got %q, want %q", got.SharedWithUserID, share.SharedWithUserID)
	}
	if got.SharedByUserID != share.SharedByUserID {
		t.Errorf("SharedByUserID: got %q, want %q", got.SharedByUserID, share.SharedByUserID)
	}
	if got.Permission != domain.PermissionWrite {
		t.Errorf("Permission: got %v, want %v", got.Permission, domain.PermissionWrite)
	}
	if got.DeletedAt != nil {
		t.Error("DeletedAt: expected nil")
	}

	// Timestamps should round-trip.
	if got.CreatedAt.Unix() != share.CreatedAt.Unix() {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, share.CreatedAt)
	}
	if got.UpdatedAt.Unix() != share.UpdatedAt.Unix() {
		t.Errorf("UpdatedAt: got %v, want %v", got.UpdatedAt, share.UpdatedAt)
	}
}

func TestGetSharesForUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "owner-1")
	insertTestUser(t, s, "friend-1")
	insertTestLibrary(t, s, "lib-1", "owner-1")

	now := time.Now()
	// Create two collections.
	for _, id := range []string{"coll-su-1", "coll-su-2"} {
		coll := &domain.Collection{
			CreatedAt: now,
			UpdatedAt: now,
			ID:        id,
			LibraryID: "lib-1",
			OwnerID:   "owner-1",
			Name:      "Collection " + id,
		}
		if err := s.CreateCollection(ctx, coll); err != nil {
			t.Fatalf("CreateCollection %s: %v", id, err)
		}
	}

	// Share both collections with the same user.
	share1 := &domain.CollectionShare{
		Syncable: domain.Syncable{
			ID:        "share-su-1",
			CreatedAt: now,
			UpdatedAt: now,
		},
		CollectionID:     "coll-su-1",
		SharedWithUserID: "friend-1",
		SharedByUserID:   "owner-1",
		Permission:       domain.PermissionRead,
	}
	share2 := &domain.CollectionShare{
		Syncable: domain.Syncable{
			ID:        "share-su-2",
			CreatedAt: now,
			UpdatedAt: now,
		},
		CollectionID:     "coll-su-2",
		SharedWithUserID: "friend-1",
		SharedByUserID:   "owner-1",
		Permission:       domain.PermissionWrite,
	}

	if err := s.CreateShare(ctx, share1); err != nil {
		t.Fatalf("CreateShare 1: %v", err)
	}
	if err := s.CreateShare(ctx, share2); err != nil {
		t.Fatalf("CreateShare 2: %v", err)
	}

	shares, err := s.GetSharesForUser(ctx, "friend-1")
	if err != nil {
		t.Fatalf("GetSharesForUser: %v", err)
	}

	if len(shares) != 2 {
		t.Fatalf("got %d shares, want 2", len(shares))
	}

	idSet := map[string]bool{}
	for _, sh := range shares {
		idSet[sh.ID] = true
	}
	if !idSet["share-su-1"] {
		t.Error("missing share-su-1")
	}
	if !idSet["share-su-2"] {
		t.Error("missing share-su-2")
	}
}

func TestDeleteShare(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestUser(t, s, "owner-1")
	insertTestUser(t, s, "friend-1")
	insertTestLibrary(t, s, "lib-1", "owner-1")

	now := time.Now()
	coll := &domain.Collection{
		CreatedAt: now,
		UpdatedAt: now,
		ID:        "coll-ds",
		LibraryID: "lib-1",
		OwnerID:   "owner-1",
		Name:      "Delete Share Test",
	}
	if err := s.CreateCollection(ctx, coll); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	share := &domain.CollectionShare{
		Syncable: domain.Syncable{
			ID:        "share-del",
			CreatedAt: now,
			UpdatedAt: now,
		},
		CollectionID:     "coll-ds",
		SharedWithUserID: "friend-1",
		SharedByUserID:   "owner-1",
		Permission:       domain.PermissionRead,
	}

	if err := s.CreateShare(ctx, share); err != nil {
		t.Fatalf("CreateShare: %v", err)
	}

	// Verify it exists.
	_, err := s.GetShare(ctx, "share-del")
	if err != nil {
		t.Fatalf("GetShare before delete: %v", err)
	}

	// Soft delete.
	if err := s.DeleteShare(ctx, "share-del"); err != nil {
		t.Fatalf("DeleteShare: %v", err)
	}

	// GetShare should return not found (soft-deleted).
	_, err = s.GetShare(ctx, "share-del")
	if err == nil {
		t.Fatal("expected not found after soft delete, got nil")
	}
	var storeErr *store.Error
	if !errors.As(err, &storeErr) {
		t.Fatalf("expected *store.Error, got %T: %v", err, err)
	}
	if storeErr.Code != store.ErrNotFound.Code {
		t.Errorf("expected status %d, got %d", store.ErrNotFound.Code, storeErr.Code)
	}

	// GetSharesForUser should also exclude the soft-deleted share.
	shares, err := s.GetSharesForUser(ctx, "friend-1")
	if err != nil {
		t.Fatalf("GetSharesForUser: %v", err)
	}
	for _, sh := range shares {
		if sh.ID == "share-del" {
			t.Error("soft-deleted share should not appear in GetSharesForUser")
		}
	}

	// GetSharesForCollection should also exclude it.
	collShares, err := s.GetSharesForCollection(ctx, "coll-ds")
	if err != nil {
		t.Fatalf("GetSharesForCollection: %v", err)
	}
	for _, sh := range collShares {
		if sh.ID == "share-del" {
			t.Error("soft-deleted share should not appear in GetSharesForCollection")
		}
	}
}
