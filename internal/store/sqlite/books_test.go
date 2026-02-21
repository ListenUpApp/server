package sqlite

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// makeTestBook creates a domain.Book with sensible defaults for testing.
func makeTestBook(id, title, path string) *domain.Book {
	now := time.Now()
	return &domain.Book{
		Syncable: domain.Syncable{
			ID:        id,
			CreatedAt: now,
			UpdatedAt: now,
		},
		ScannedAt:     now,
		Title:         title,
		Path:          path,
		TotalDuration: 36000,
		TotalSize:     500000000,
		StagedCollectionIDs: []string{},
	}
}

func TestCreateAndGetBook(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	book := makeTestBook("book-1", "The Hobbit", "/audiobooks/the-hobbit")
	book.ISBN = "978-0-261-10295-4"
	book.Subtitle = "There and Back Again"
	book.Description = "A children's fantasy novel"
	book.Publisher = "George Allen & Unwin"
	book.PublishYear = "1937"
	book.Language = "en"
	book.ASIN = "B009RRXELG"
	book.AudibleRegion = "us"
	book.Abridged = true
	book.CoverImage = &domain.ImageFileInfo{
		Path:     "/audiobooks/the-hobbit/cover.jpg",
		Filename: "cover.jpg",
		Format:   "jpeg",
		Size:     150000,
		Inode:    12345,
		ModTime:  1700000000,
		BlurHash: "LEHV6nWB2yk8",
	}
	book.AudioFiles = []domain.AudioFileInfo{
		{
			ID:       "af-1",
			Path:     "/audiobooks/the-hobbit/chapter01.mp3",
			Filename: "chapter01.mp3",
			Format:   "mp3",
			Codec:    "mp3",
			Size:     50000000,
			Duration: 3600,
			Bitrate:  128000,
			Inode:    100,
			ModTime:  1700000000,
		},
		{
			ID:       "af-2",
			Path:     "/audiobooks/the-hobbit/chapter02.mp3",
			Filename: "chapter02.mp3",
			Format:   "mp3",
			Codec:    "mp3",
			Size:     60000000,
			Duration: 4200,
			Bitrate:  128000,
			Inode:    101,
			ModTime:  1700000000,
		},
	}
	book.Chapters = []domain.Chapter{
		{
			Index:       0,
			Title:       "An Unexpected Party",
			AudioFileID: "af-1",
			StartTime:   0,
			EndTime:     3600,
		},
		{
			Index:       1,
			Title:       "Roast Mutton",
			AudioFileID: "af-2",
			StartTime:   0,
			EndTime:     4200,
		},
	}

	if err := s.CreateBook(ctx, book); err != nil {
		t.Fatalf("CreateBook: %v", err)
	}

	got, err := s.GetBook(ctx, "book-1")
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}

	// Verify scalar fields.
	if got.ID != book.ID {
		t.Errorf("ID: got %q, want %q", got.ID, book.ID)
	}
	if got.Title != book.Title {
		t.Errorf("Title: got %q, want %q", got.Title, book.Title)
	}
	if got.Subtitle != book.Subtitle {
		t.Errorf("Subtitle: got %q, want %q", got.Subtitle, book.Subtitle)
	}
	if got.Path != book.Path {
		t.Errorf("Path: got %q, want %q", got.Path, book.Path)
	}
	if got.ISBN != book.ISBN {
		t.Errorf("ISBN: got %q, want %q", got.ISBN, book.ISBN)
	}
	if got.Description != book.Description {
		t.Errorf("Description: got %q, want %q", got.Description, book.Description)
	}
	if got.Publisher != book.Publisher {
		t.Errorf("Publisher: got %q, want %q", got.Publisher, book.Publisher)
	}
	if got.PublishYear != book.PublishYear {
		t.Errorf("PublishYear: got %q, want %q", got.PublishYear, book.PublishYear)
	}
	if got.Language != book.Language {
		t.Errorf("Language: got %q, want %q", got.Language, book.Language)
	}
	if got.ASIN != book.ASIN {
		t.Errorf("ASIN: got %q, want %q", got.ASIN, book.ASIN)
	}
	if got.AudibleRegion != book.AudibleRegion {
		t.Errorf("AudibleRegion: got %q, want %q", got.AudibleRegion, book.AudibleRegion)
	}
	if got.TotalDuration != book.TotalDuration {
		t.Errorf("TotalDuration: got %d, want %d", got.TotalDuration, book.TotalDuration)
	}
	if got.TotalSize != book.TotalSize {
		t.Errorf("TotalSize: got %d, want %d", got.TotalSize, book.TotalSize)
	}
	if !got.Abridged {
		t.Error("Abridged: expected true")
	}
	if got.DeletedAt != nil {
		t.Error("DeletedAt: expected nil")
	}

	// Timestamps should round-trip through RFC3339Nano.
	if got.CreatedAt.Unix() != book.CreatedAt.Unix() {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, book.CreatedAt)
	}
	if got.UpdatedAt.Unix() != book.UpdatedAt.Unix() {
		t.Errorf("UpdatedAt: got %v, want %v", got.UpdatedAt, book.UpdatedAt)
	}
	if got.ScannedAt.Unix() != book.ScannedAt.Unix() {
		t.Errorf("ScannedAt: got %v, want %v", got.ScannedAt, book.ScannedAt)
	}

	// Verify cover image.
	if got.CoverImage == nil {
		t.Fatal("CoverImage: expected non-nil")
	}
	if got.CoverImage.Path != book.CoverImage.Path {
		t.Errorf("CoverImage.Path: got %q, want %q", got.CoverImage.Path, book.CoverImage.Path)
	}
	if got.CoverImage.Filename != book.CoverImage.Filename {
		t.Errorf("CoverImage.Filename: got %q, want %q", got.CoverImage.Filename, book.CoverImage.Filename)
	}
	if got.CoverImage.Format != book.CoverImage.Format {
		t.Errorf("CoverImage.Format: got %q, want %q", got.CoverImage.Format, book.CoverImage.Format)
	}
	if got.CoverImage.Size != book.CoverImage.Size {
		t.Errorf("CoverImage.Size: got %d, want %d", got.CoverImage.Size, book.CoverImage.Size)
	}
	if got.CoverImage.Inode != book.CoverImage.Inode {
		t.Errorf("CoverImage.Inode: got %d, want %d", got.CoverImage.Inode, book.CoverImage.Inode)
	}
	if got.CoverImage.ModTime != book.CoverImage.ModTime {
		t.Errorf("CoverImage.ModTime: got %d, want %d", got.CoverImage.ModTime, book.CoverImage.ModTime)
	}
	if got.CoverImage.BlurHash != book.CoverImage.BlurHash {
		t.Errorf("CoverImage.BlurHash: got %q, want %q", got.CoverImage.BlurHash, book.CoverImage.BlurHash)
	}

	// Verify audio files.
	if len(got.AudioFiles) != 2 {
		t.Fatalf("AudioFiles: got %d, want 2", len(got.AudioFiles))
	}
	af := got.AudioFiles[0]
	if af.ID != "af-1" {
		t.Errorf("AudioFile[0].ID: got %q, want %q", af.ID, "af-1")
	}
	if af.Path != "/audiobooks/the-hobbit/chapter01.mp3" {
		t.Errorf("AudioFile[0].Path: got %q", af.Path)
	}
	if af.Codec != "mp3" {
		t.Errorf("AudioFile[0].Codec: got %q, want %q", af.Codec, "mp3")
	}
	if af.Bitrate != 128000 {
		t.Errorf("AudioFile[0].Bitrate: got %d, want %d", af.Bitrate, 128000)
	}
	if af.Inode != 100 {
		t.Errorf("AudioFile[0].Inode: got %d, want %d", af.Inode, 100)
	}

	// Verify chapters.
	if len(got.Chapters) != 2 {
		t.Fatalf("Chapters: got %d, want 2", len(got.Chapters))
	}
	ch := got.Chapters[0]
	if ch.Title != "An Unexpected Party" {
		t.Errorf("Chapter[0].Title: got %q", ch.Title)
	}
	if ch.AudioFileID != "af-1" {
		t.Errorf("Chapter[0].AudioFileID: got %q, want %q", ch.AudioFileID, "af-1")
	}
	if ch.StartTime != 0 || ch.EndTime != 3600 {
		t.Errorf("Chapter[0].Times: got %d-%d, want 0-3600", ch.StartTime, ch.EndTime)
	}

	// Contributors and Series should be nil (not loaded in book queries).
	if got.Contributors != nil {
		t.Error("Contributors: expected nil")
	}
	if got.Series != nil {
		t.Error("Series: expected nil")
	}
}

func TestGetBook_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetBook(ctx, "nonexistent")
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

func TestCreateBook_DuplicatePath(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	b1 := makeTestBook("book-dup-1", "Book One", "/audiobooks/same-path")
	if err := s.CreateBook(ctx, b1); err != nil {
		t.Fatalf("CreateBook b1: %v", err)
	}

	// Different ID, same path.
	b2 := makeTestBook("book-dup-2", "Book Two", "/audiobooks/same-path")
	err := s.CreateBook(ctx, b2)
	if err == nil {
		t.Fatal("expected error for duplicate path, got nil")
	}
	if !errors.Is(err, store.ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestGetBookByPath(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	book := makeTestBook("book-path", "Path Test", "/audiobooks/by-path")
	if err := s.CreateBook(ctx, book); err != nil {
		t.Fatalf("CreateBook: %v", err)
	}

	got, err := s.GetBookByPath(ctx, "/audiobooks/by-path")
	if err != nil {
		t.Fatalf("GetBookByPath: %v", err)
	}
	if got.ID != "book-path" {
		t.Errorf("ID: got %q, want %q", got.ID, "book-path")
	}
	if got.Title != "Path Test" {
		t.Errorf("Title: got %q, want %q", got.Title, "Path Test")
	}
}

func TestGetBookByPath_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetBookByPath(ctx, "/nonexistent/path")
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

func TestListBooks_Pagination(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create 5 books with staggered updated_at times.
	for i := 1; i <= 5; i++ {
		b := makeTestBook(
			fmt.Sprintf("book-list-%d", i),
			fmt.Sprintf("Book %d", i),
			fmt.Sprintf("/audiobooks/book-%d", i),
		)
		// Stagger updated_at so ordering is deterministic.
		b.UpdatedAt = b.UpdatedAt.Add(time.Duration(i) * time.Second)
		if err := s.CreateBook(ctx, b); err != nil {
			t.Fatalf("CreateBook(%d): %v", i, err)
		}
	}

	// First page: limit 2.
	params := store.PaginationParams{Limit: 2}
	page1, err := s.ListBooks(ctx, params)
	if err != nil {
		t.Fatalf("ListBooks page1: %v", err)
	}

	if len(page1.Items) != 2 {
		t.Fatalf("page1: got %d items, want 2", len(page1.Items))
	}
	if !page1.HasMore {
		t.Error("page1: expected HasMore=true")
	}
	if page1.Total != 5 {
		t.Errorf("page1: Total got %d, want 5", page1.Total)
	}
	if page1.NextCursor == "" {
		t.Fatal("page1: expected non-empty NextCursor")
	}
	if page1.Items[0].ID != "book-list-1" {
		t.Errorf("page1[0].ID: got %q, want %q", page1.Items[0].ID, "book-list-1")
	}
	if page1.Items[1].ID != "book-list-2" {
		t.Errorf("page1[1].ID: got %q, want %q", page1.Items[1].ID, "book-list-2")
	}

	// Second page.
	params.Cursor = page1.NextCursor
	page2, err := s.ListBooks(ctx, params)
	if err != nil {
		t.Fatalf("ListBooks page2: %v", err)
	}

	if len(page2.Items) != 2 {
		t.Fatalf("page2: got %d items, want 2", len(page2.Items))
	}
	if !page2.HasMore {
		t.Error("page2: expected HasMore=true")
	}
	if page2.Items[0].ID != "book-list-3" {
		t.Errorf("page2[0].ID: got %q, want %q", page2.Items[0].ID, "book-list-3")
	}
	if page2.Items[1].ID != "book-list-4" {
		t.Errorf("page2[1].ID: got %q, want %q", page2.Items[1].ID, "book-list-4")
	}

	// Third page: only 1 remaining.
	params.Cursor = page2.NextCursor
	page3, err := s.ListBooks(ctx, params)
	if err != nil {
		t.Fatalf("ListBooks page3: %v", err)
	}

	if len(page3.Items) != 1 {
		t.Fatalf("page3: got %d items, want 1", len(page3.Items))
	}
	if page3.HasMore {
		t.Error("page3: expected HasMore=false")
	}
	if page3.Items[0].ID != "book-list-5" {
		t.Errorf("page3[0].ID: got %q, want %q", page3.Items[0].ID, "book-list-5")
	}
}

func TestUpdateBook(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	book := makeTestBook("book-update", "Original Title", "/audiobooks/update-test")
	book.AudioFiles = []domain.AudioFileInfo{
		{
			ID: "af-orig", Path: "/audiobooks/update-test/orig.mp3",
			Filename: "orig.mp3", Format: "mp3", Size: 1000, Duration: 100,
			Inode: 200, ModTime: 1700000000,
		},
	}
	book.Chapters = []domain.Chapter{
		{Index: 0, Title: "Original Chapter", StartTime: 0, EndTime: 100},
	}

	if err := s.CreateBook(ctx, book); err != nil {
		t.Fatalf("CreateBook: %v", err)
	}

	// Modify fields.
	book.Title = "Updated Title"
	book.Subtitle = "A New Subtitle"
	book.Description = "Updated description"
	book.Abridged = true
	book.CoverImage = &domain.ImageFileInfo{
		Path: "/audiobooks/update-test/cover.jpg", Filename: "cover.jpg",
		Format: "jpeg", Size: 200000, Inode: 999, ModTime: 1700001000,
		BlurHash: "NEWBLURHASH",
	}
	book.AudioFiles = []domain.AudioFileInfo{
		{
			ID: "af-new-1", Path: "/audiobooks/update-test/new1.mp3",
			Filename: "new1.mp3", Format: "mp3", Codec: "mp3",
			Size: 2000, Duration: 200, Bitrate: 64000,
			Inode: 300, ModTime: 1700001000,
		},
		{
			ID: "af-new-2", Path: "/audiobooks/update-test/new2.mp3",
			Filename: "new2.mp3", Format: "mp3", Codec: "mp3",
			Size: 3000, Duration: 300, Bitrate: 64000,
			Inode: 301, ModTime: 1700001000,
		},
	}
	book.Chapters = []domain.Chapter{
		{Index: 0, Title: "New Chapter 1", AudioFileID: "af-new-1", StartTime: 0, EndTime: 200},
		{Index: 1, Title: "New Chapter 2", AudioFileID: "af-new-2", StartTime: 0, EndTime: 300},
	}
	book.Touch()

	if err := s.UpdateBook(ctx, book); err != nil {
		t.Fatalf("UpdateBook: %v", err)
	}

	got, err := s.GetBook(ctx, "book-update")
	if err != nil {
		t.Fatalf("GetBook after update: %v", err)
	}

	if got.Title != "Updated Title" {
		t.Errorf("Title: got %q, want %q", got.Title, "Updated Title")
	}
	if got.Subtitle != "A New Subtitle" {
		t.Errorf("Subtitle: got %q, want %q", got.Subtitle, "A New Subtitle")
	}
	if got.Description != "Updated description" {
		t.Errorf("Description: got %q, want %q", got.Description, "Updated description")
	}
	if !got.Abridged {
		t.Error("Abridged: expected true")
	}
	if got.CoverImage == nil {
		t.Fatal("CoverImage: expected non-nil after update")
	}
	if got.CoverImage.BlurHash != "NEWBLURHASH" {
		t.Errorf("CoverImage.BlurHash: got %q, want %q", got.CoverImage.BlurHash, "NEWBLURHASH")
	}

	// Audio files should be replaced.
	if len(got.AudioFiles) != 2 {
		t.Fatalf("AudioFiles: got %d, want 2", len(got.AudioFiles))
	}
	if got.AudioFiles[0].ID != "af-new-1" {
		t.Errorf("AudioFile[0].ID: got %q, want %q", got.AudioFiles[0].ID, "af-new-1")
	}
	if got.AudioFiles[1].ID != "af-new-2" {
		t.Errorf("AudioFile[1].ID: got %q, want %q", got.AudioFiles[1].ID, "af-new-2")
	}

	// Chapters should be replaced.
	if len(got.Chapters) != 2 {
		t.Fatalf("Chapters: got %d, want 2", len(got.Chapters))
	}
	if got.Chapters[0].Title != "New Chapter 1" {
		t.Errorf("Chapter[0].Title: got %q", got.Chapters[0].Title)
	}
	if got.Chapters[1].Title != "New Chapter 2" {
		t.Errorf("Chapter[1].Title: got %q", got.Chapters[1].Title)
	}
}

func TestUpdateBook_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	book := makeTestBook("nonexistent-book", "Ghost", "/audiobooks/ghost")

	err := s.UpdateBook(ctx, book)
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

func TestDeleteBook(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	book := makeTestBook("book-delete", "Delete Me", "/audiobooks/delete-test")
	if err := s.CreateBook(ctx, book); err != nil {
		t.Fatalf("CreateBook: %v", err)
	}

	// Verify book exists before delete.
	_, err := s.GetBook(ctx, "book-delete")
	if err != nil {
		t.Fatalf("GetBook before delete: %v", err)
	}

	// Soft delete.
	if err := s.DeleteBook(ctx, "book-delete"); err != nil {
		t.Fatalf("DeleteBook: %v", err)
	}

	// GetBook should return not found.
	_, err = s.GetBook(ctx, "book-delete")
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

	// Deleting again should return not found (already deleted).
	err = s.DeleteBook(ctx, "book-delete")
	if err == nil {
		t.Fatal("expected not found on double delete, got nil")
	}
	if !errors.As(err, &storeErr) {
		t.Fatalf("expected *store.Error on double delete, got %T: %v", err, err)
	}
}

func TestDeleteBook_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	err := s.DeleteBook(ctx, "never-existed")
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

func TestGetBooksForSync(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	baseline := time.Now().Add(-10 * time.Second)

	// Create a book with updated_at in the past (before baseline).
	oldBook := makeTestBook("book-old", "Old Book", "/audiobooks/old")
	oldBook.CreatedAt = baseline.Add(-5 * time.Second)
	oldBook.UpdatedAt = baseline.Add(-5 * time.Second)
	oldBook.ScannedAt = baseline.Add(-5 * time.Second)
	if err := s.CreateBook(ctx, oldBook); err != nil {
		t.Fatalf("CreateBook old: %v", err)
	}

	// Create a book with updated_at after baseline.
	newBook := makeTestBook("book-new", "New Book", "/audiobooks/new")
	newBook.CreatedAt = baseline.Add(2 * time.Second)
	newBook.UpdatedAt = baseline.Add(2 * time.Second)
	newBook.ScannedAt = baseline.Add(2 * time.Second)
	newBook.AudioFiles = []domain.AudioFileInfo{
		{
			ID: "af-sync", Path: "/audiobooks/new/track.mp3",
			Filename: "track.mp3", Format: "mp3", Size: 1000, Duration: 100,
			Inode: 500, ModTime: 1700000000,
		},
	}
	if err := s.CreateBook(ctx, newBook); err != nil {
		t.Fatalf("CreateBook new: %v", err)
	}

	// Query for books updated after baseline.
	books, err := s.GetBooksForSync(ctx, baseline)
	if err != nil {
		t.Fatalf("GetBooksForSync: %v", err)
	}

	if len(books) != 1 {
		t.Fatalf("GetBooksForSync: got %d books, want 1", len(books))
	}
	if books[0].ID != "book-new" {
		t.Errorf("ID: got %q, want %q", books[0].ID, "book-new")
	}

	// Verify audio files are loaded.
	if len(books[0].AudioFiles) != 1 {
		t.Errorf("AudioFiles: got %d, want 1", len(books[0].AudioFiles))
	}
}

func TestGetBooksDeletedAfter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	baseline := time.Now()

	// Create and delete a book.
	book := makeTestBook("book-del-after", "Deleted Book", "/audiobooks/del-after")
	if err := s.CreateBook(ctx, book); err != nil {
		t.Fatalf("CreateBook: %v", err)
	}

	// Small sleep to ensure deleted_at > baseline.
	time.Sleep(10 * time.Millisecond)

	if err := s.DeleteBook(ctx, "book-del-after"); err != nil {
		t.Fatalf("DeleteBook: %v", err)
	}

	// Create another book that is NOT deleted.
	alive := makeTestBook("book-alive", "Alive Book", "/audiobooks/alive")
	if err := s.CreateBook(ctx, alive); err != nil {
		t.Fatalf("CreateBook alive: %v", err)
	}

	ids, err := s.GetBooksDeletedAfter(ctx, baseline)
	if err != nil {
		t.Fatalf("GetBooksDeletedAfter: %v", err)
	}

	if len(ids) != 1 {
		t.Fatalf("GetBooksDeletedAfter: got %d ids, want 1", len(ids))
	}
	if ids[0] != "book-del-after" {
		t.Errorf("ID: got %q, want %q", ids[0], "book-del-after")
	}
}

func TestBook_NilCoverImage(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	book := makeTestBook("book-no-cover", "No Cover", "/audiobooks/no-cover")
	// CoverImage is nil by default.

	if err := s.CreateBook(ctx, book); err != nil {
		t.Fatalf("CreateBook: %v", err)
	}

	got, err := s.GetBook(ctx, "book-no-cover")
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}

	if got.CoverImage != nil {
		t.Errorf("CoverImage: expected nil, got %+v", got.CoverImage)
	}
}

func TestBook_StagedCollectionIDs(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	book := makeTestBook("book-staged", "Staged Book", "/audiobooks/staged")
	book.StagedCollectionIDs = []string{"coll-1", "coll-2", "coll-3"}

	if err := s.CreateBook(ctx, book); err != nil {
		t.Fatalf("CreateBook: %v", err)
	}

	got, err := s.GetBook(ctx, "book-staged")
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}

	if len(got.StagedCollectionIDs) != 3 {
		t.Fatalf("StagedCollectionIDs: got %d, want 3", len(got.StagedCollectionIDs))
	}
	expected := []string{"coll-1", "coll-2", "coll-3"}
	for i, id := range got.StagedCollectionIDs {
		if id != expected[i] {
			t.Errorf("StagedCollectionIDs[%d]: got %q, want %q", i, id, expected[i])
		}
	}

	// Update with different staged IDs.
	book.StagedCollectionIDs = []string{"coll-x"}
	book.Touch()
	if err := s.UpdateBook(ctx, book); err != nil {
		t.Fatalf("UpdateBook: %v", err)
	}

	got, err = s.GetBook(ctx, "book-staged")
	if err != nil {
		t.Fatalf("GetBook after update: %v", err)
	}

	if len(got.StagedCollectionIDs) != 1 {
		t.Fatalf("StagedCollectionIDs after update: got %d, want 1", len(got.StagedCollectionIDs))
	}
	if got.StagedCollectionIDs[0] != "coll-x" {
		t.Errorf("StagedCollectionIDs[0]: got %q, want %q", got.StagedCollectionIDs[0], "coll-x")
	}
}
