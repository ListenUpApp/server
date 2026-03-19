package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
)

func TestBatchWriter_FlushWritesJunctionTables(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a contributor and series first (like ConvertToBook does via extractContributors/extractSeries).
	contributor, err := s.GetOrCreateContributor(ctx, "J.R.R. Tolkien")
	if err != nil {
		t.Fatalf("GetOrCreateContributor: %v", err)
	}

	series, err := s.GetOrCreateSeries(ctx, "Middle-earth")
	if err != nil {
		t.Fatalf("GetOrCreateSeries: %v", err)
	}

	// Create a genre (like the genre seed does).
	now := time.Now()
	genre := &domain.Genre{
		Syncable: domain.Syncable{
			ID:        "genre-fantasy",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:     "Fantasy",
		Slug:     "fantasy",
		Path:     "/fantasy",
		IsSystem: true,
	}
	if err := s.CreateGenre(ctx, genre); err != nil {
		t.Fatalf("CreateGenre: %v", err)
	}

	// Create a book with contributors, series, and genres — exactly like scanner does.
	book := makeTestBook("batch-junc-1", "The Hobbit", "/books/hobbit")
	book.Contributors = []domain.BookContributor{
		{
			ContributorID: contributor.ID,
			Roles:         []domain.ContributorRole{domain.RoleAuthor},
		},
	}
	book.Series = []domain.BookSeries{
		{
			SeriesID: series.ID,
			Sequence: "1",
		},
	}
	book.GenreIDs = []string{"genre-fantasy"}

	// Use BatchWriter exactly like the scanner does.
	bw := s.NewBatchWriter(100)
	if err := bw.CreateBook(ctx, book); err != nil {
		t.Fatalf("CreateBook: %v", err)
	}
	if err := bw.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// Verify book was created.
	got, err := s.GetBook(ctx, "batch-junc-1", "")
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	if got.Title != "The Hobbit" {
		t.Errorf("book title: got %q, want %q", got.Title, "The Hobbit")
	}

	// Verify contributor junction table was written.
	contribs, err := s.GetBookContributors(ctx, "batch-junc-1")
	if err != nil {
		t.Fatalf("GetBookContributors: %v", err)
	}
	if len(contribs) != 1 {
		t.Errorf("expected 1 contributor, got %d", len(contribs))
	} else if contribs[0].ContributorID != contributor.ID {
		t.Errorf("contributor ID: got %q, want %q", contribs[0].ContributorID, contributor.ID)
	}

	// Verify series junction table was written.
	bookSeries, err := s.GetBookSeries(ctx, "batch-junc-1")
	if err != nil {
		t.Fatalf("GetBookSeries: %v", err)
	}
	if len(bookSeries) != 1 {
		t.Errorf("expected 1 series, got %d", len(bookSeries))
	} else if bookSeries[0].SeriesID != series.ID {
		t.Errorf("series ID: got %q, want %q", bookSeries[0].SeriesID, series.ID)
	}

	// Verify genre junction table was written.
	genreIDs, err := s.GetGenreIDsForBook(ctx, "batch-junc-1")
	if err != nil {
		t.Fatalf("GetGenreIDsForBook: %v", err)
	}
	if len(genreIDs) != 1 {
		t.Errorf("expected 1 genre, got %d", len(genreIDs))
	} else if genreIDs[0] != "genre-fantasy" {
		t.Errorf("genre ID: got %q, want %q", genreIDs[0], "genre-fantasy")
	}
}

// TestBatchWriter_FlushUpdatesJunctionTablesForExistingBook verifies that when a
// book already exists in the database (ErrAlreadyExists), the batch writer still
// writes junction table relationships. This covers the case where books were
// originally created before junction-table writing was added to createBookTx.
func TestBatchWriter_FlushUpdatesJunctionTablesForExistingBook(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a contributor and series.
	contributor, err := s.GetOrCreateContributor(ctx, "Brandon Sanderson")
	if err != nil {
		t.Fatalf("GetOrCreateContributor: %v", err)
	}

	series, err := s.GetOrCreateSeries(ctx, "Stormlight Archive")
	if err != nil {
		t.Fatalf("GetOrCreateSeries: %v", err)
	}

	now := time.Now()
	genre := &domain.Genre{
		Syncable: domain.Syncable{
			ID:        "genre-epic-fantasy",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:     "Epic Fantasy",
		Slug:     "epic-fantasy",
		Path:     "/epic-fantasy",
		IsSystem: true,
	}
	if err := s.CreateGenre(ctx, genre); err != nil {
		t.Fatalf("CreateGenre: %v", err)
	}

	// Step 1: Create book WITHOUT junction tables (simulates old code).
	bookNoJunctions := makeTestBook("batch-existing-1", "The Way of Kings", "/books/twok")
	if err := s.CreateBook(ctx, bookNoJunctions); err != nil {
		t.Fatalf("CreateBook: %v", err)
	}

	// Verify junction tables are empty.
	contribs, _ := s.GetBookContributors(ctx, "batch-existing-1")
	if len(contribs) != 0 {
		t.Fatalf("expected 0 contributors before fix, got %d", len(contribs))
	}
	bookSeries, _ := s.GetBookSeries(ctx, "batch-existing-1")
	if len(bookSeries) != 0 {
		t.Fatalf("expected 0 series before fix, got %d", len(bookSeries))
	}

	// Step 2: Try to create the same book via batch writer WITH junction data.
	// This simulates a rescan after the junction-table fix was deployed.
	bookWithJunctions := makeTestBook("batch-existing-1", "The Way of Kings", "/books/twok")
	bookWithJunctions.Contributors = []domain.BookContributor{
		{
			ContributorID: contributor.ID,
			Roles:         []domain.ContributorRole{domain.RoleAuthor},
		},
	}
	bookWithJunctions.Series = []domain.BookSeries{
		{
			SeriesID: series.ID,
			Sequence: "1",
		},
	}
	bookWithJunctions.GenreIDs = []string{"genre-epic-fantasy"}

	bw := s.NewBatchWriter(100)
	if err := bw.CreateBook(ctx, bookWithJunctions); err != nil {
		t.Fatalf("CreateBook: %v", err)
	}
	if err := bw.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// Step 3: Verify junction tables were written despite ErrAlreadyExists.
	contribs, err = s.GetBookContributors(ctx, "batch-existing-1")
	if err != nil {
		t.Fatalf("GetBookContributors: %v", err)
	}
	if len(contribs) != 1 {
		t.Errorf("expected 1 contributor after fix, got %d", len(contribs))
	} else if contribs[0].ContributorID != contributor.ID {
		t.Errorf("contributor ID: got %q, want %q", contribs[0].ContributorID, contributor.ID)
	}

	bookSeries, err = s.GetBookSeries(ctx, "batch-existing-1")
	if err != nil {
		t.Fatalf("GetBookSeries: %v", err)
	}
	if len(bookSeries) != 1 {
		t.Errorf("expected 1 series after fix, got %d", len(bookSeries))
	} else if bookSeries[0].SeriesID != series.ID {
		t.Errorf("series ID: got %q, want %q", bookSeries[0].SeriesID, series.ID)
	}

	genreIDs, err := s.GetGenreIDsForBook(ctx, "batch-existing-1")
	if err != nil {
		t.Fatalf("GetGenreIDsForBook: %v", err)
	}
	if len(genreIDs) != 1 {
		t.Errorf("expected 1 genre after fix, got %d", len(genreIDs))
	} else if genreIDs[0] != "genre-epic-fantasy" {
		t.Errorf("genre ID: got %q, want %q", genreIDs[0], "genre-epic-fantasy")
	}
}

func TestUpdateBookJunctionTables_BumpsUpdatedAt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a contributor.
	contributor, err := s.GetOrCreateContributor(ctx, "Patrick Rothfuss")
	if err != nil {
		t.Fatalf("GetOrCreateContributor: %v", err)
	}

	// Create a book with a known timestamp in the past.
	book := makeTestBook("bump-test-1", "The Name of the Wind", "/books/notw")
	book.UpdatedAt = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	book.CreatedAt = book.UpdatedAt
	if err := s.CreateBook(ctx, book); err != nil {
		t.Fatalf("CreateBook: %v", err)
	}

	// Record the original updated_at.
	original, err := s.GetBook(ctx, "bump-test-1", "")
	if err != nil {
		t.Fatalf("GetBook before: %v", err)
	}
	originalUpdatedAt := original.UpdatedAt

	// Call updateBookJunctionTables with contributor data.
	book.Contributors = []domain.BookContributor{
		{
			ContributorID: contributor.ID,
			Roles:         []domain.ContributorRole{domain.RoleAuthor},
		},
	}
	if err := s.updateBookJunctionTables(ctx, book); err != nil {
		t.Fatalf("updateBookJunctionTables: %v", err)
	}

	// Verify updated_at was bumped.
	after, err := s.GetBook(ctx, "bump-test-1", "")
	if err != nil {
		t.Fatalf("GetBook after: %v", err)
	}
	if !after.UpdatedAt.After(originalUpdatedAt) {
		t.Errorf("expected updated_at to be bumped: before=%v, after=%v", originalUpdatedAt, after.UpdatedAt)
	}
}
