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

// makeTestContributor creates a domain.Contributor with sensible defaults for testing.
func makeTestContributor(id, name string) *domain.Contributor {
	now := time.Now()
	return &domain.Contributor{
		Syncable: domain.Syncable{
			ID:        id,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:     name,
		SortName: name,
		Aliases:  []string{},
	}
}

func TestCreateAndGetContributor(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	c := makeTestContributor("contrib-1", "Brandon Sanderson")
	c.SortName = "Sanderson, Brandon"
	c.Biography = "American author of epic fantasy and science fiction."
	c.ImageURL = "https://example.com/sanderson.jpg"
	c.ImageBlurHash = "LEHV6nWB2yk8"
	c.ASIN = "B001IGFHW6"
	c.Aliases = []string{"Brando Sando"}
	c.Website = "https://brandonsanderson.com"
	c.BirthDate = "1975-12-19"
	c.DeathDate = ""

	if err := s.CreateContributor(ctx, c); err != nil {
		t.Fatalf("CreateContributor: %v", err)
	}

	got, err := s.GetContributor(ctx, "contrib-1")
	if err != nil {
		t.Fatalf("GetContributor: %v", err)
	}

	// Verify fields.
	if got.ID != c.ID {
		t.Errorf("ID: got %q, want %q", got.ID, c.ID)
	}
	if got.Name != c.Name {
		t.Errorf("Name: got %q, want %q", got.Name, c.Name)
	}
	if got.SortName != "Sanderson, Brandon" {
		t.Errorf("SortName: got %q, want %q", got.SortName, "Sanderson, Brandon")
	}
	if got.Biography != c.Biography {
		t.Errorf("Biography: got %q, want %q", got.Biography, c.Biography)
	}
	if got.ImageURL != c.ImageURL {
		t.Errorf("ImageURL: got %q, want %q", got.ImageURL, c.ImageURL)
	}
	if got.ImageBlurHash != c.ImageBlurHash {
		t.Errorf("ImageBlurHash: got %q, want %q", got.ImageBlurHash, c.ImageBlurHash)
	}
	if got.ASIN != c.ASIN {
		t.Errorf("ASIN: got %q, want %q", got.ASIN, c.ASIN)
	}
	if len(got.Aliases) != 1 || got.Aliases[0] != "Brando Sando" {
		t.Errorf("Aliases: got %v, want %v", got.Aliases, c.Aliases)
	}
	if got.Website != c.Website {
		t.Errorf("Website: got %q, want %q", got.Website, c.Website)
	}
	if got.BirthDate != c.BirthDate {
		t.Errorf("BirthDate: got %q, want %q", got.BirthDate, c.BirthDate)
	}
	if got.DeathDate != "" {
		t.Errorf("DeathDate: got %q, want empty", got.DeathDate)
	}
	if got.DeletedAt != nil {
		t.Error("DeletedAt: expected nil")
	}

	// Timestamps should round-trip through RFC3339Nano.
	if got.CreatedAt.Unix() != c.CreatedAt.Unix() {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, c.CreatedAt)
	}
	if got.UpdatedAt.Unix() != c.UpdatedAt.Unix() {
		t.Errorf("UpdatedAt: got %v, want %v", got.UpdatedAt, c.UpdatedAt)
	}
}

func TestGetContributor_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetContributor(ctx, "nonexistent")
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

func TestCreateContributor_Duplicate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	c1 := makeTestContributor("contrib-dup", "Stephen King")
	if err := s.CreateContributor(ctx, c1); err != nil {
		t.Fatalf("CreateContributor c1: %v", err)
	}

	// Same ID should fail.
	c2 := makeTestContributor("contrib-dup", "Richard Bachman")
	err := s.CreateContributor(ctx, c2)
	if err == nil {
		t.Fatal("expected error for duplicate ID, got nil")
	}
	if !errors.Is(err, store.ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestListContributors_Pagination(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create 5 contributors with sort_names that produce a known alphabetical order.
	names := []struct {
		id       string
		name     string
		sortName string
	}{
		{"c-1", "Brandon Sanderson", "Sanderson, Brandon"},
		{"c-2", "Stephen King", "King, Stephen"},
		{"c-3", "Joe Abercrombie", "Abercrombie, Joe"},
		{"c-4", "Robin Hobb", "Hobb, Robin"},
		{"c-5", "Patrick Rothfuss", "Rothfuss, Patrick"},
	}
	// Expected alphabetical order by sort_name: Abercrombie, Hobb, King, Rothfuss, Sanderson

	for _, n := range names {
		c := makeTestContributor(n.id, n.name)
		c.SortName = n.sortName
		if err := s.CreateContributor(ctx, c); err != nil {
			t.Fatalf("CreateContributor(%s): %v", n.id, err)
		}
	}

	// Page 1: limit 2
	params := store.PaginationParams{Limit: 2}
	page1, err := s.ListContributors(ctx, params)
	if err != nil {
		t.Fatalf("ListContributors page 1: %v", err)
	}
	if len(page1.Items) != 2 {
		t.Fatalf("page 1: got %d items, want 2", len(page1.Items))
	}
	if page1.Total != 5 {
		t.Errorf("page 1 total: got %d, want 5", page1.Total)
	}
	if !page1.HasMore {
		t.Error("page 1: expected HasMore=true")
	}
	if page1.Items[0].SortName != "Abercrombie, Joe" {
		t.Errorf("page 1 item 0: got %q, want %q", page1.Items[0].SortName, "Abercrombie, Joe")
	}
	if page1.Items[1].SortName != "Hobb, Robin" {
		t.Errorf("page 1 item 1: got %q, want %q", page1.Items[1].SortName, "Hobb, Robin")
	}

	// Page 2: use cursor from page 1
	params.Cursor = page1.NextCursor
	page2, err := s.ListContributors(ctx, params)
	if err != nil {
		t.Fatalf("ListContributors page 2: %v", err)
	}
	if len(page2.Items) != 2 {
		t.Fatalf("page 2: got %d items, want 2", len(page2.Items))
	}
	if !page2.HasMore {
		t.Error("page 2: expected HasMore=true")
	}
	if page2.Items[0].SortName != "King, Stephen" {
		t.Errorf("page 2 item 0: got %q, want %q", page2.Items[0].SortName, "King, Stephen")
	}
	if page2.Items[1].SortName != "Rothfuss, Patrick" {
		t.Errorf("page 2 item 1: got %q, want %q", page2.Items[1].SortName, "Rothfuss, Patrick")
	}

	// Page 3: last page
	params.Cursor = page2.NextCursor
	page3, err := s.ListContributors(ctx, params)
	if err != nil {
		t.Fatalf("ListContributors page 3: %v", err)
	}
	if len(page3.Items) != 1 {
		t.Fatalf("page 3: got %d items, want 1", len(page3.Items))
	}
	if page3.HasMore {
		t.Error("page 3: expected HasMore=false")
	}
	if page3.Items[0].SortName != "Sanderson, Brandon" {
		t.Errorf("page 3 item 0: got %q, want %q", page3.Items[0].SortName, "Sanderson, Brandon")
	}
	if page3.NextCursor != "" {
		t.Errorf("page 3: expected empty NextCursor, got %q", page3.NextCursor)
	}
}

func TestUpdateContributor(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	c := makeTestContributor("contrib-update", "Robert Jordan")
	c.SortName = "Jordan, Robert"
	if err := s.CreateContributor(ctx, c); err != nil {
		t.Fatalf("CreateContributor: %v", err)
	}

	// Modify fields.
	c.Name = "Robert Jordan (James Oliver Rigney Jr.)"
	c.SortName = "Jordan, Robert"
	c.Biography = "American author of The Wheel of Time."
	c.ImageURL = "https://example.com/jordan.jpg"
	c.ASIN = "B000APCH4A"
	c.Aliases = []string{"Reagan O'Neal", "Jackson O'Reilly"}
	c.Website = "https://dragonmount.com"
	c.BirthDate = "1948-10-17"
	c.DeathDate = "2007-09-16"
	c.Touch()

	if err := s.UpdateContributor(ctx, c); err != nil {
		t.Fatalf("UpdateContributor: %v", err)
	}

	got, err := s.GetContributor(ctx, "contrib-update")
	if err != nil {
		t.Fatalf("GetContributor after update: %v", err)
	}

	if got.Name != c.Name {
		t.Errorf("Name: got %q, want %q", got.Name, c.Name)
	}
	if got.Biography != c.Biography {
		t.Errorf("Biography: got %q, want %q", got.Biography, c.Biography)
	}
	if got.ImageURL != c.ImageURL {
		t.Errorf("ImageURL: got %q, want %q", got.ImageURL, c.ImageURL)
	}
	if got.ASIN != c.ASIN {
		t.Errorf("ASIN: got %q, want %q", got.ASIN, c.ASIN)
	}
	if len(got.Aliases) != 2 {
		t.Fatalf("Aliases: got %d items, want 2", len(got.Aliases))
	}
	if got.Aliases[0] != "Reagan O'Neal" || got.Aliases[1] != "Jackson O'Reilly" {
		t.Errorf("Aliases: got %v, want %v", got.Aliases, c.Aliases)
	}
	if got.Website != c.Website {
		t.Errorf("Website: got %q, want %q", got.Website, c.Website)
	}
	if got.BirthDate != c.BirthDate {
		t.Errorf("BirthDate: got %q, want %q", got.BirthDate, c.BirthDate)
	}
	if got.DeathDate != c.DeathDate {
		t.Errorf("DeathDate: got %q, want %q", got.DeathDate, c.DeathDate)
	}
}

func TestUpdateContributor_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	c := makeTestContributor("nonexistent-contrib", "Nobody")

	err := s.UpdateContributor(ctx, c)
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

func TestGetOrCreateContributor_Creates(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	got, err := s.GetOrCreateContributor(ctx, "Terry Pratchett")
	if err != nil {
		t.Fatalf("GetOrCreateContributor: %v", err)
	}

	if got.ID == "" {
		t.Error("expected non-empty ID")
	}
	if got.Name != "Terry Pratchett" {
		t.Errorf("Name: got %q, want %q", got.Name, "Terry Pratchett")
	}
	if got.SortName != "Terry Pratchett" {
		t.Errorf("SortName: got %q, want %q", got.SortName, "Terry Pratchett")
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt: expected non-zero")
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt: expected non-zero")
	}

	// Verify it was persisted.
	fetched, err := s.GetContributor(ctx, got.ID)
	if err != nil {
		t.Fatalf("GetContributor after create: %v", err)
	}
	if fetched.Name != "Terry Pratchett" {
		t.Errorf("persisted Name: got %q, want %q", fetched.Name, "Terry Pratchett")
	}
}

func TestGetOrCreateContributor_Finds(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Pre-create contributor.
	original := makeTestContributor("contrib-existing", "Neil Gaiman")
	original.SortName = "Gaiman, Neil"
	original.Biography = "English author."
	if err := s.CreateContributor(ctx, original); err != nil {
		t.Fatalf("CreateContributor: %v", err)
	}

	// GetOrCreate with different case should find the existing one.
	cases := []string{"neil gaiman", "NEIL GAIMAN", "Neil Gaiman", "nEiL gAiMaN"}
	for _, name := range cases {
		got, err := s.GetOrCreateContributor(ctx, name)
		if err != nil {
			t.Fatalf("GetOrCreateContributor(%q): %v", name, err)
		}
		if got.ID != "contrib-existing" {
			t.Errorf("GetOrCreateContributor(%q): ID = %q, want %q", name, got.ID, "contrib-existing")
		}
		if got.Biography != "English author." {
			t.Errorf("GetOrCreateContributor(%q): Biography = %q, want %q", name, got.Biography, "English author.")
		}
	}
}

func TestContributor_AliasesRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		aliases []string
	}{
		{"empty aliases", []string{}},
		{"single alias", []string{"Richard Bachman"}},
		{"multiple aliases", []string{"Richard Bachman", "John Swithen", "Beryl Evans"}},
		{"special characters", []string{`O'Brien`, `"The Master"`, "Anne Rice / A.N. Roquelaure"}},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			id := fmt.Sprintf("alias-test-%d", i)
			c := makeTestContributor(id, fmt.Sprintf("Author %d", i))
			c.Aliases = tc.aliases

			if err := s.CreateContributor(ctx, c); err != nil {
				t.Fatalf("CreateContributor: %v", err)
			}

			got, err := s.GetContributor(ctx, id)
			if err != nil {
				t.Fatalf("GetContributor: %v", err)
			}

			if len(got.Aliases) != len(tc.aliases) {
				t.Fatalf("Aliases length: got %d, want %d", len(got.Aliases), len(tc.aliases))
			}
			for j, alias := range tc.aliases {
				if got.Aliases[j] != alias {
					t.Errorf("Aliases[%d]: got %q, want %q", j, got.Aliases[j], alias)
				}
			}
		})
	}
}

func TestContributor_SoftDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	c := makeTestContributor("contrib-soft-del", "Iain Banks")
	c.SortName = "Banks, Iain"
	if err := s.CreateContributor(ctx, c); err != nil {
		t.Fatalf("CreateContributor: %v", err)
	}

	// Verify it exists.
	_, err := s.GetContributor(ctx, "contrib-soft-del")
	if err != nil {
		t.Fatalf("GetContributor before delete: %v", err)
	}

	// Soft delete by setting deleted_at directly.
	now := formatTime(time.Now())
	_, err = s.db.ExecContext(ctx,
		`UPDATE contributors SET deleted_at = ?, updated_at = ? WHERE id = ?`,
		now, now, "contrib-soft-del")
	if err != nil {
		t.Fatalf("soft delete exec: %v", err)
	}

	// GetContributor should return not found.
	_, err = s.GetContributor(ctx, "contrib-soft-del")
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

	// ListContributors should not include the soft-deleted contributor.
	params := store.PaginationParams{Limit: 100}
	result, err := s.ListContributors(ctx, params)
	if err != nil {
		t.Fatalf("ListContributors: %v", err)
	}
	for _, item := range result.Items {
		if item.ID == "contrib-soft-del" {
			t.Error("soft-deleted contributor should not appear in list")
		}
	}
	if result.Total != 0 {
		t.Errorf("Total: got %d, want 0 (only soft-deleted contributor exists)", result.Total)
	}

	// UpdateContributor should return not found for soft-deleted record.
	c.Name = "Updated Name"
	c.Touch()
	err = s.UpdateContributor(ctx, c)
	if err == nil {
		t.Fatal("expected not found on update of soft-deleted, got nil")
	}
	if !errors.As(err, &storeErr) {
		t.Fatalf("expected *store.Error, got %T: %v", err, err)
	}
	if storeErr.Code != store.ErrNotFound.Code {
		t.Errorf("expected status %d, got %d", store.ErrNotFound.Code, storeErr.Code)
	}
}
