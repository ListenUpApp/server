package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
)

func insertTestBook(t *testing.T, s *Store, id, title, path string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(`INSERT INTO books (id, created_at, updated_at, scanned_at, title, path) VALUES (?,?,?,?,?,?)`,
		id, now, now, now, title, path)
	if err != nil {
		t.Fatalf("insert test book: %v", err)
	}
}

func insertTestContributor(t *testing.T, s *Store, id, name string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(`INSERT INTO contributors (id, created_at, updated_at, name) VALUES (?,?,?,?)`,
		id, now, now, name)
	if err != nil {
		t.Fatalf("insert test contributor: %v", err)
	}
}

func TestSetAndGetBookContributors(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestBook(t, s, "book-1", "The Way of Kings", "/books/twok")
	insertTestContributor(t, s, "contrib-1", "Brandon Sanderson")
	insertTestContributor(t, s, "contrib-2", "Michael Kramer")

	contributors := []domain.BookContributor{
		{
			ContributorID: "contrib-1",
			Roles:         []domain.ContributorRole{domain.RoleAuthor},
			CreditedAs:    "Brandon Sanderson",
		},
		{
			ContributorID: "contrib-2",
			Roles:         []domain.ContributorRole{domain.RoleNarrator},
			CreditedAs:    "",
		},
	}

	if err := s.SetBookContributors(ctx, "book-1", contributors); err != nil {
		t.Fatalf("SetBookContributors: %v", err)
	}

	got, err := s.GetBookContributors(ctx, "book-1")
	if err != nil {
		t.Fatalf("GetBookContributors: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 contributors, got %d", len(got))
	}

	// Build a map for order-independent comparison.
	byID := make(map[string]domain.BookContributor)
	for _, c := range got {
		byID[c.ContributorID] = c
	}

	// Verify contributor 1.
	c1, ok := byID["contrib-1"]
	if !ok {
		t.Fatal("contrib-1 not found in results")
	}
	if len(c1.Roles) != 1 || c1.Roles[0] != domain.RoleAuthor {
		t.Errorf("contrib-1 roles: got %v, want [author]", c1.Roles)
	}
	if c1.CreditedAs != "Brandon Sanderson" {
		t.Errorf("contrib-1 credited_as: got %q, want %q", c1.CreditedAs, "Brandon Sanderson")
	}

	// Verify contributor 2.
	c2, ok := byID["contrib-2"]
	if !ok {
		t.Fatal("contrib-2 not found in results")
	}
	if len(c2.Roles) != 1 || c2.Roles[0] != domain.RoleNarrator {
		t.Errorf("contrib-2 roles: got %v, want [narrator]", c2.Roles)
	}
	if c2.CreditedAs != "" {
		t.Errorf("contrib-2 credited_as: got %q, want empty", c2.CreditedAs)
	}
}

func TestSetBookContributors_Replace(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestBook(t, s, "book-r", "Words of Radiance", "/books/wor")
	insertTestContributor(t, s, "contrib-a", "Brandon Sanderson")
	insertTestContributor(t, s, "contrib-b", "Michael Kramer")
	insertTestContributor(t, s, "contrib-c", "Kate Reading")

	// First set: contrib-a and contrib-b.
	first := []domain.BookContributor{
		{ContributorID: "contrib-a", Roles: []domain.ContributorRole{domain.RoleAuthor}},
		{ContributorID: "contrib-b", Roles: []domain.ContributorRole{domain.RoleNarrator}},
	}
	if err := s.SetBookContributors(ctx, "book-r", first); err != nil {
		t.Fatalf("SetBookContributors (first): %v", err)
	}

	// Replace with: contrib-a and contrib-c.
	second := []domain.BookContributor{
		{ContributorID: "contrib-a", Roles: []domain.ContributorRole{domain.RoleAuthor}},
		{ContributorID: "contrib-c", Roles: []domain.ContributorRole{domain.RoleNarrator}, CreditedAs: "Kate Reading"},
	}
	if err := s.SetBookContributors(ctx, "book-r", second); err != nil {
		t.Fatalf("SetBookContributors (second): %v", err)
	}

	got, err := s.GetBookContributors(ctx, "book-r")
	if err != nil {
		t.Fatalf("GetBookContributors: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 contributors after replace, got %d", len(got))
	}

	byID := make(map[string]domain.BookContributor)
	for _, c := range got {
		byID[c.ContributorID] = c
	}

	if _, ok := byID["contrib-b"]; ok {
		t.Error("contrib-b should have been removed after replace")
	}

	if c, ok := byID["contrib-c"]; !ok {
		t.Error("contrib-c not found after replace")
	} else if c.CreditedAs != "Kate Reading" {
		t.Errorf("contrib-c credited_as: got %q, want %q", c.CreditedAs, "Kate Reading")
	}

	if _, ok := byID["contrib-a"]; !ok {
		t.Error("contrib-a should still be present after replace")
	}
}

func TestGetBookContributors_Empty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestBook(t, s, "book-empty", "Oathbringer", "/books/ob")

	got, err := s.GetBookContributors(ctx, "book-empty")
	if err != nil {
		t.Fatalf("GetBookContributors: %v", err)
	}

	if got != nil {
		t.Errorf("expected nil for empty contributors, got %v", got)
	}
}

func TestSetBookContributors_MultipleRoles(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestBook(t, s, "book-mr", "Rhythm of War", "/books/row")
	insertTestContributor(t, s, "contrib-multi", "Neil Gaiman")

	contributors := []domain.BookContributor{
		{
			ContributorID: "contrib-multi",
			Roles:         []domain.ContributorRole{domain.RoleAuthor, domain.RoleNarrator},
			CreditedAs:    "Neil Gaiman",
		},
	}

	if err := s.SetBookContributors(ctx, "book-mr", contributors); err != nil {
		t.Fatalf("SetBookContributors: %v", err)
	}

	got, err := s.GetBookContributors(ctx, "book-mr")
	if err != nil {
		t.Fatalf("GetBookContributors: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 contributor, got %d", len(got))
	}

	if len(got[0].Roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(got[0].Roles))
	}

	roleSet := make(map[domain.ContributorRole]bool)
	for _, r := range got[0].Roles {
		roleSet[r] = true
	}

	if !roleSet[domain.RoleAuthor] {
		t.Error("expected author role to be present")
	}
	if !roleSet[domain.RoleNarrator] {
		t.Error("expected narrator role to be present")
	}

	if got[0].CreditedAs != "Neil Gaiman" {
		t.Errorf("credited_as: got %q, want %q", got[0].CreditedAs, "Neil Gaiman")
	}
}
