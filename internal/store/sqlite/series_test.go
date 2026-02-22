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

// makeTestSeries creates a domain.Series with sensible defaults for testing.
func makeTestSeries(id, name string) *domain.Series {
	now := time.Now()
	return &domain.Series{
		Syncable: domain.Syncable{
			ID:        id,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:        name,
		Description: "A test series",
		ASIN:        "B00TEST1234",
		CoverImage: &domain.ImageFileInfo{
			Path:     "/covers/series/" + id + ".jpg",
			Filename: id + ".jpg",
			Format:   "jpeg",
			Size:     12345,
			Inode:    98765,
			ModTime:  now.Unix(),
			BlurHash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
		},
	}
}

func TestCreateAndGetSeries(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	series := makeTestSeries("series-1", "The Wheel of Time")

	if err := s.CreateSeries(ctx, series); err != nil {
		t.Fatalf("CreateSeries: %v", err)
	}

	got, err := s.GetSeries(ctx, "series-1")
	if err != nil {
		t.Fatalf("GetSeries: %v", err)
	}

	// Verify fields.
	if got.ID != series.ID {
		t.Errorf("ID: got %q, want %q", got.ID, series.ID)
	}
	if got.Name != series.Name {
		t.Errorf("Name: got %q, want %q", got.Name, series.Name)
	}
	if got.Description != series.Description {
		t.Errorf("Description: got %q, want %q", got.Description, series.Description)
	}
	if got.ASIN != series.ASIN {
		t.Errorf("ASIN: got %q, want %q", got.ASIN, series.ASIN)
	}
	if got.DeletedAt != nil {
		t.Error("DeletedAt: expected nil")
	}

	// Timestamps should round-trip through RFC3339Nano.
	if got.CreatedAt.Unix() != series.CreatedAt.Unix() {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, series.CreatedAt)
	}
	if got.UpdatedAt.Unix() != series.UpdatedAt.Unix() {
		t.Errorf("UpdatedAt: got %v, want %v", got.UpdatedAt, series.UpdatedAt)
	}

	// Verify cover image.
	if got.CoverImage == nil {
		t.Fatal("CoverImage: expected non-nil")
	}
	if got.CoverImage.Path != series.CoverImage.Path {
		t.Errorf("CoverImage.Path: got %q, want %q", got.CoverImage.Path, series.CoverImage.Path)
	}
	if got.CoverImage.Filename != series.CoverImage.Filename {
		t.Errorf("CoverImage.Filename: got %q, want %q", got.CoverImage.Filename, series.CoverImage.Filename)
	}
	if got.CoverImage.Format != series.CoverImage.Format {
		t.Errorf("CoverImage.Format: got %q, want %q", got.CoverImage.Format, series.CoverImage.Format)
	}
	if got.CoverImage.Size != series.CoverImage.Size {
		t.Errorf("CoverImage.Size: got %d, want %d", got.CoverImage.Size, series.CoverImage.Size)
	}
	if got.CoverImage.Inode != series.CoverImage.Inode {
		t.Errorf("CoverImage.Inode: got %d, want %d", got.CoverImage.Inode, series.CoverImage.Inode)
	}
	if got.CoverImage.ModTime != series.CoverImage.ModTime {
		t.Errorf("CoverImage.ModTime: got %d, want %d", got.CoverImage.ModTime, series.CoverImage.ModTime)
	}
	if got.CoverImage.BlurHash != series.CoverImage.BlurHash {
		t.Errorf("CoverImage.BlurHash: got %q, want %q", got.CoverImage.BlurHash, series.CoverImage.BlurHash)
	}
}

func TestGetSeries_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetSeries(ctx, "nonexistent")
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

func TestCreateSeries_Duplicate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	s1 := makeTestSeries("series-dup", "Mistborn")
	if err := s.CreateSeries(ctx, s1); err != nil {
		t.Fatalf("CreateSeries s1: %v", err)
	}

	// Same ID, different name.
	s2 := makeTestSeries("series-dup", "Mistborn Era 2")
	err := s.CreateSeries(ctx, s2)
	if err == nil {
		t.Fatal("expected error for duplicate ID, got nil")
	}
	if !errors.Is(err, store.ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestListSeries_Pagination(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create 5 series with names that sort case-insensitively.
	names := []string{"Echo", "alpha", "Charlie", "bravo", "Delta"}
	for i, name := range names {
		series := makeTestSeries(fmt.Sprintf("series-%d", i+1), name)
		if err := s.CreateSeries(ctx, series); err != nil {
			t.Fatalf("CreateSeries(%s): %v", name, err)
		}
	}

	// Expected case-insensitive sort order: alpha, bravo, Charlie, Delta, Echo
	// IDs in that order: series-2, series-4, series-3, series-5, series-1

	// Page 1: limit 2.
	params := store.PaginationParams{Limit: 2}
	page1, err := s.ListSeries(ctx, params)
	if err != nil {
		t.Fatalf("ListSeries page 1: %v", err)
	}

	if page1.Total != 5 {
		t.Errorf("Total: got %d, want 5", page1.Total)
	}
	if len(page1.Items) != 2 {
		t.Fatalf("page 1 items: got %d, want 2", len(page1.Items))
	}
	if page1.Items[0].Name != "alpha" {
		t.Errorf("page 1 item 0: got %q, want %q", page1.Items[0].Name, "alpha")
	}
	if page1.Items[1].Name != "bravo" {
		t.Errorf("page 1 item 1: got %q, want %q", page1.Items[1].Name, "bravo")
	}
	if !page1.HasMore {
		t.Error("page 1: expected HasMore=true")
	}
	if page1.NextCursor == "" {
		t.Fatal("page 1: expected non-empty NextCursor")
	}

	// Page 2: use cursor from page 1.
	params2 := store.PaginationParams{Limit: 2, Cursor: page1.NextCursor}
	page2, err := s.ListSeries(ctx, params2)
	if err != nil {
		t.Fatalf("ListSeries page 2: %v", err)
	}

	if len(page2.Items) != 2 {
		t.Fatalf("page 2 items: got %d, want 2", len(page2.Items))
	}
	if page2.Items[0].Name != "Charlie" {
		t.Errorf("page 2 item 0: got %q, want %q", page2.Items[0].Name, "Charlie")
	}
	if page2.Items[1].Name != "Delta" {
		t.Errorf("page 2 item 1: got %q, want %q", page2.Items[1].Name, "Delta")
	}
	if !page2.HasMore {
		t.Error("page 2: expected HasMore=true")
	}

	// Page 3: last page.
	params3 := store.PaginationParams{Limit: 2, Cursor: page2.NextCursor}
	page3, err := s.ListSeries(ctx, params3)
	if err != nil {
		t.Fatalf("ListSeries page 3: %v", err)
	}

	if len(page3.Items) != 1 {
		t.Fatalf("page 3 items: got %d, want 1", len(page3.Items))
	}
	if page3.Items[0].Name != "Echo" {
		t.Errorf("page 3 item 0: got %q, want %q", page3.Items[0].Name, "Echo")
	}
	if page3.HasMore {
		t.Error("page 3: expected HasMore=false")
	}
}

func TestUpdateSeries(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	series := makeTestSeries("series-update", "The Stormlight Archive")
	if err := s.CreateSeries(ctx, series); err != nil {
		t.Fatalf("CreateSeries: %v", err)
	}

	// Modify fields.
	series.Name = "Stormlight Archive"
	series.Description = "Updated description"
	series.ASIN = "B00UPDATED"
	series.CoverImage = &domain.ImageFileInfo{
		Path:     "/covers/updated.jpg",
		Filename: "updated.jpg",
		Format:   "png",
		Size:     99999,
		Inode:    11111,
		ModTime:  time.Now().Unix(),
		BlurHash: "LKO2:N%2Tw=w]~RBVZRi};RPxuwH",
	}
	series.Touch()

	if err := s.UpdateSeries(ctx, series); err != nil {
		t.Fatalf("UpdateSeries: %v", err)
	}

	got, err := s.GetSeries(ctx, "series-update")
	if err != nil {
		t.Fatalf("GetSeries after update: %v", err)
	}

	if got.Name != "Stormlight Archive" {
		t.Errorf("Name: got %q, want %q", got.Name, "Stormlight Archive")
	}
	if got.Description != "Updated description" {
		t.Errorf("Description: got %q, want %q", got.Description, "Updated description")
	}
	if got.ASIN != "B00UPDATED" {
		t.Errorf("ASIN: got %q, want %q", got.ASIN, "B00UPDATED")
	}
	if got.CoverImage == nil {
		t.Fatal("CoverImage: expected non-nil after update")
	}
	if got.CoverImage.Path != "/covers/updated.jpg" {
		t.Errorf("CoverImage.Path: got %q, want %q", got.CoverImage.Path, "/covers/updated.jpg")
	}
	if got.CoverImage.Format != "png" {
		t.Errorf("CoverImage.Format: got %q, want %q", got.CoverImage.Format, "png")
	}
	if got.CoverImage.Size != 99999 {
		t.Errorf("CoverImage.Size: got %d, want %d", got.CoverImage.Size, 99999)
	}
}

func TestUpdateSeries_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	series := makeTestSeries("nonexistent-series", "Ghost Series")

	err := s.UpdateSeries(ctx, series)
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

func TestSeries_NilCoverImage(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	series := makeTestSeries("series-nocover", "No Cover Series")
	series.CoverImage = nil

	if err := s.CreateSeries(ctx, series); err != nil {
		t.Fatalf("CreateSeries: %v", err)
	}

	got, err := s.GetSeries(ctx, "series-nocover")
	if err != nil {
		t.Fatalf("GetSeries: %v", err)
	}

	if got.CoverImage != nil {
		t.Errorf("CoverImage: expected nil, got %+v", got.CoverImage)
	}
	if got.Name != "No Cover Series" {
		t.Errorf("Name: got %q, want %q", got.Name, "No Cover Series")
	}
}

func TestSeries_SoftDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	series := makeTestSeries("series-delete", "Doomed Series")
	if err := s.CreateSeries(ctx, series); err != nil {
		t.Fatalf("CreateSeries: %v", err)
	}

	// Verify series exists before soft delete.
	_, err := s.GetSeries(ctx, "series-delete")
	if err != nil {
		t.Fatalf("GetSeries before delete: %v", err)
	}

	// Soft delete by setting deleted_at directly.
	now := formatTime(time.Now())
	_, err = s.db.ExecContext(ctx,
		`UPDATE series SET deleted_at = ?, updated_at = ? WHERE id = ?`,
		now, now, "series-delete")
	if err != nil {
		t.Fatalf("soft delete exec: %v", err)
	}

	// GetSeries should return not found.
	_, err = s.GetSeries(ctx, "series-delete")
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

	// ListSeries should also exclude the soft-deleted series.
	result, err := s.ListSeries(ctx, store.PaginationParams{Limit: 100})
	if err != nil {
		t.Fatalf("ListSeries: %v", err)
	}
	for _, item := range result.Items {
		if item.ID == "series-delete" {
			t.Error("ListSeries returned soft-deleted series")
		}
	}

	// UpdateSeries should also return not found for soft-deleted series.
	series.Touch()
	err = s.UpdateSeries(ctx, series)
	if err == nil {
		t.Fatal("expected not found on update of soft-deleted series, got nil")
	}
}
