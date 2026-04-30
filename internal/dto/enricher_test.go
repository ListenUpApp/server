package dto

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/listenupapp/listenup-server/internal/domain"
)

// fakeStore is a test double for the Store interface used by Enricher.
//
// It returns canned data from in-memory maps and counts how many times each
// method was called so tests can assert the no-N+1 property of EnrichBooks.
type fakeStore struct {
	contributors map[string]*domain.Contributor
	series       map[string]*domain.Series
	genres       map[string]*domain.Genre
	tagsByBook   map[string][]*domain.Tag

	// Error injection.
	contributorsErr error
	seriesErr       error
	genresErr       error
	tagsErr         error

	// Call counters.
	getContributorsByIDsCalls int
	getSeriesByIDsCalls       int
	getSeriesCalls            int
	getGenresByIDsCalls       int
	getTagsForBookCalls       int
	getTagsForBookIDsCalls    int

	// Last argument tracking (useful for asserting batched IDs).
	lastContributorIDs []string
	lastSeriesIDs      []string
	lastGenreIDs       []string
	lastTagBookIDs     []string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		contributors: map[string]*domain.Contributor{},
		series:       map[string]*domain.Series{},
		genres:       map[string]*domain.Genre{},
		tagsByBook:   map[string][]*domain.Tag{},
	}
}

func (f *fakeStore) GetContributorsByIDs(_ context.Context, ids []string) ([]*domain.Contributor, error) {
	f.getContributorsByIDsCalls++
	f.lastContributorIDs = append([]string(nil), ids...)
	if f.contributorsErr != nil {
		return nil, f.contributorsErr
	}
	out := make([]*domain.Contributor, 0, len(ids))
	for _, id := range ids {
		if c, ok := f.contributors[id]; ok {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeStore) GetGenresByIDs(_ context.Context, ids []string) ([]*domain.Genre, error) {
	f.getGenresByIDsCalls++
	f.lastGenreIDs = append([]string(nil), ids...)
	if f.genresErr != nil {
		return nil, f.genresErr
	}
	out := make([]*domain.Genre, 0, len(ids))
	for _, id := range ids {
		if g, ok := f.genres[id]; ok {
			out = append(out, g)
		}
	}
	return out, nil
}

func (f *fakeStore) GetSeries(_ context.Context, id string) (*domain.Series, error) {
	f.getSeriesCalls++
	if f.seriesErr != nil {
		return nil, f.seriesErr
	}
	if s, ok := f.series[id]; ok {
		return s, nil
	}
	return nil, errors.New("not found")
}

func (f *fakeStore) GetSeriesByIDs(_ context.Context, ids []string) ([]*domain.Series, error) {
	f.getSeriesByIDsCalls++
	f.lastSeriesIDs = append([]string(nil), ids...)
	if f.seriesErr != nil {
		return nil, f.seriesErr
	}
	out := make([]*domain.Series, 0, len(ids))
	for _, id := range ids {
		if s, ok := f.series[id]; ok {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeStore) GetTagsForBook(_ context.Context, bookID string) ([]*domain.Tag, error) {
	f.getTagsForBookCalls++
	if f.tagsErr != nil {
		return nil, f.tagsErr
	}
	return f.tagsByBook[bookID], nil
}

func (f *fakeStore) GetTagsForBookIDs(_ context.Context, bookIDs []string) (map[string][]*domain.Tag, error) {
	f.getTagsForBookIDsCalls++
	f.lastTagBookIDs = append([]string(nil), bookIDs...)
	if f.tagsErr != nil {
		return nil, f.tagsErr
	}
	out := make(map[string][]*domain.Tag, len(bookIDs))
	for _, id := range bookIDs {
		if tags, ok := f.tagsByBook[id]; ok {
			out[id] = tags
		}
	}
	return out, nil
}

// makeBook is a small helper to build a domain.Book for tests.
func makeBook(id, title string, contribs []domain.BookContributor, series []domain.BookSeries, genreIDs []string) *domain.Book {
	return &domain.Book{
		Syncable: domain.Syncable{
			ID:        id,
			CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		Title:        title,
		Contributors: contribs,
		Series:       series,
		GenreIDs:     genreIDs,
	}
}

// === EnrichBook tests =====================================================

func TestEnrichBook(t *testing.T) {
	t.Parallel()

	type setupFn func(s *fakeStore)
	type assertFn func(t *testing.T, book *Book)

	tests := []struct {
		name    string
		book    *domain.Book
		setup   setupFn
		assert  assertFn
		wantErr bool
	}{
		{
			name: "happy path: all references resolve",
			book: makeBook("b1", "Book One",
				[]domain.BookContributor{
					{ContributorID: "c1", Roles: []domain.ContributorRole{domain.RoleAuthor}},
					{ContributorID: "c2", Roles: []domain.ContributorRole{domain.RoleNarrator}},
				},
				[]domain.BookSeries{{SeriesID: "s1", Sequence: "1"}},
				[]string{"g1"},
			),
			setup: func(s *fakeStore) {
				s.contributors["c1"] = &domain.Contributor{Syncable: domain.Syncable{ID: "c1"}, Name: "Alice Author"}
				s.contributors["c2"] = &domain.Contributor{Syncable: domain.Syncable{ID: "c2"}, Name: "Norman Narrator"}
				s.series["s1"] = &domain.Series{Syncable: domain.Syncable{ID: "s1"}, Name: "Saga"}
				s.genres["g1"] = &domain.Genre{Syncable: domain.Syncable{ID: "g1"}, Name: "Fantasy"}
				s.tagsByBook["b1"] = []*domain.Tag{{ID: "t1", Slug: "slow-burn", BookCount: 3}}
			},
			assert: func(t *testing.T, book *Book) {
				t.Helper()
				assert.Equal(t, "Alice Author", book.Author)
				assert.Equal(t, "Norman Narrator", book.Narrator)
				require.Len(t, book.Contributors, 2)
				assert.Equal(t, "Alice Author", book.Contributors[0].Name)
				assert.Equal(t, []string{"author"}, book.Contributors[0].Roles)
				require.Len(t, book.SeriesInfo, 1)
				assert.Equal(t, "Saga", book.SeriesInfo[0].Name)
				assert.Equal(t, "1", book.SeriesInfo[0].Sequence)
				assert.Equal(t, "Saga", book.SeriesName)
				assert.Equal(t, []string{"Fantasy"}, book.Genres)
				require.Len(t, book.Tags, 1)
				assert.Equal(t, "slow-burn", book.Tags[0].Slug)
			},
		},
		{
			name: "missing contributor leaves name empty but keeps ID and roles",
			book: makeBook("b1", "Book",
				[]domain.BookContributor{
					{ContributorID: "c1", Roles: []domain.ContributorRole{domain.RoleAuthor}},
					{ContributorID: "c-missing", Roles: []domain.ContributorRole{domain.RoleNarrator}},
				},
				nil, nil,
			),
			setup: func(s *fakeStore) {
				s.contributors["c1"] = &domain.Contributor{Syncable: domain.Syncable{ID: "c1"}, Name: "Alice Author"}
			},
			assert: func(t *testing.T, book *Book) {
				t.Helper()
				assert.Equal(t, "Alice Author", book.Author)
				assert.Empty(t, book.Narrator, "narrator name remains empty when contributor not found")
				require.Len(t, book.Contributors, 2)
				assert.Equal(t, "c-missing", book.Contributors[1].ContributorID)
				assert.Empty(t, book.Contributors[1].Name)
				assert.Equal(t, []string{"narrator"}, book.Contributors[1].Roles)
			},
		},
		{
			name: "missing series is dropped from SeriesInfo",
			book: makeBook("b1", "Book",
				nil,
				[]domain.BookSeries{
					{SeriesID: "s1", Sequence: "1"},
					{SeriesID: "s-missing", Sequence: "2"},
				},
				nil,
			),
			setup: func(s *fakeStore) {
				s.series["s1"] = &domain.Series{Syncable: domain.Syncable{ID: "s1"}, Name: "Real Series"}
			},
			assert: func(t *testing.T, book *Book) {
				t.Helper()
				require.Len(t, book.SeriesInfo, 1)
				assert.Equal(t, "Real Series", book.SeriesInfo[0].Name)
				assert.Equal(t, "Real Series", book.SeriesName)
			},
		},
		{
			name: "multi-role contributor is both author and narrator",
			book: makeBook("b1", "Book",
				[]domain.BookContributor{
					{
						ContributorID: "c1",
						Roles:         []domain.ContributorRole{domain.RoleAuthor, domain.RoleNarrator},
					},
				},
				nil, nil,
			),
			setup: func(s *fakeStore) {
				s.contributors["c1"] = &domain.Contributor{Syncable: domain.Syncable{ID: "c1"}, Name: "Renaissance Person"}
			},
			assert: func(t *testing.T, book *Book) {
				t.Helper()
				assert.Equal(t, "Renaissance Person", book.Author)
				assert.Equal(t, "Renaissance Person", book.Narrator)
				require.Len(t, book.Contributors, 1)
				assert.ElementsMatch(t, []string{"author", "narrator"}, book.Contributors[0].Roles)
			},
		},
		{
			name: "empty genre list yields nil/empty Genres",
			book: makeBook("b1", "Book",
				[]domain.BookContributor{
					{ContributorID: "c1", Roles: []domain.ContributorRole{domain.RoleAuthor}},
				},
				nil, nil,
			),
			setup: func(s *fakeStore) {
				s.contributors["c1"] = &domain.Contributor{Syncable: domain.Syncable{ID: "c1"}, Name: "Author"}
			},
			assert: func(t *testing.T, book *Book) {
				t.Helper()
				assert.Empty(t, book.Genres)
			},
		},
		{
			name: "empty tag list yields nil/empty Tags",
			book: makeBook("b1", "Book",
				[]domain.BookContributor{
					{ContributorID: "c1", Roles: []domain.ContributorRole{domain.RoleAuthor}},
				},
				nil, nil,
			),
			setup: func(s *fakeStore) {
				s.contributors["c1"] = &domain.Contributor{Syncable: domain.Syncable{ID: "c1"}, Name: "Author"}
				// No tags configured for "b1".
			},
			assert: func(t *testing.T, book *Book) {
				t.Helper()
				assert.Empty(t, book.Tags)
			},
		},
		{
			name:  "book with no contributors at all",
			book:  makeBook("b1", "Book", nil, nil, nil),
			setup: func(_ *fakeStore) {},
			assert: func(t *testing.T, book *Book) {
				t.Helper()
				assert.Empty(t, book.Author)
				assert.Empty(t, book.Narrator)
				assert.Empty(t, book.Contributors)
				assert.Empty(t, book.SeriesInfo)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := newFakeStore()
			tc.setup(store)
			e := NewEnricher(store)

			got, err := e.EnrichBook(context.Background(), tc.book)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			tc.assert(t, got)
		})
	}
}

func TestEnrichBook_ContributorFetchError(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.contributorsErr = errors.New("boom")
	e := NewEnricher(store)

	book := makeBook("b1", "Book", []domain.BookContributor{
		{ContributorID: "c1", Roles: []domain.ContributorRole{domain.RoleAuthor}},
	}, nil, nil)

	_, err := e.EnrichBook(context.Background(), book)
	require.Error(t, err)
	assert.ErrorIs(t, err, store.contributorsErr)
}

// === EnrichBooks tests ====================================================

func TestEnrichBooks_EmptyInputReturnsEmptySlice(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	e := NewEnricher(store)

	got, err := e.EnrichBooks(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, got)

	assert.Zero(t, store.getContributorsByIDsCalls, "no calls for empty input")
	assert.Zero(t, store.getSeriesByIDsCalls)
	assert.Zero(t, store.getGenresByIDsCalls)
	assert.Zero(t, store.getTagsForBookIDsCalls)
}

func TestEnrichBooks_HappyPathBatched(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.contributors["c1"] = &domain.Contributor{Syncable: domain.Syncable{ID: "c1"}, Name: "Alice"}
	store.contributors["c2"] = &domain.Contributor{Syncable: domain.Syncable{ID: "c2"}, Name: "Bob"}
	store.contributors["c3"] = &domain.Contributor{Syncable: domain.Syncable{ID: "c3"}, Name: "Carol"}
	store.series["s1"] = &domain.Series{Syncable: domain.Syncable{ID: "s1"}, Name: "Saga"}
	store.genres["g1"] = &domain.Genre{Syncable: domain.Syncable{ID: "g1"}, Name: "Fantasy"}
	store.genres["g2"] = &domain.Genre{Syncable: domain.Syncable{ID: "g2"}, Name: "Mystery"}
	store.tagsByBook["b1"] = []*domain.Tag{{ID: "t1", Slug: "epic"}}
	store.tagsByBook["b2"] = []*domain.Tag{{ID: "t2", Slug: "noir"}}

	books := []*domain.Book{
		makeBook("b1", "Book 1",
			[]domain.BookContributor{
				{ContributorID: "c1", Roles: []domain.ContributorRole{domain.RoleAuthor}},
				{ContributorID: "c2", Roles: []domain.ContributorRole{domain.RoleNarrator}},
			},
			[]domain.BookSeries{{SeriesID: "s1", Sequence: "1"}},
			[]string{"g1"},
		),
		makeBook("b2", "Book 2",
			[]domain.BookContributor{
				{ContributorID: "c1", Roles: []domain.ContributorRole{domain.RoleAuthor}},
				{ContributorID: "c3", Roles: []domain.ContributorRole{domain.RoleNarrator}},
			},
			[]domain.BookSeries{{SeriesID: "s1", Sequence: "2"}},
			[]string{"g2", "g1"},
		),
		makeBook("b3", "Book 3",
			[]domain.BookContributor{
				{
					ContributorID: "c2",
					Roles:         []domain.ContributorRole{domain.RoleAuthor, domain.RoleNarrator},
				},
			},
			nil,
			nil,
		),
	}

	e := NewEnricher(store)
	got, err := e.EnrichBooks(context.Background(), books)
	require.NoError(t, err)
	require.Len(t, got, 3)

	// Spot-check denormalized fields.
	assert.Equal(t, "Alice", got[0].Author)
	assert.Equal(t, "Bob", got[0].Narrator)
	assert.Equal(t, "Saga", got[0].SeriesName)
	assert.Equal(t, []string{"Fantasy"}, got[0].Genres)
	require.Len(t, got[0].Tags, 1)
	assert.Equal(t, "epic", got[0].Tags[0].Slug)

	assert.Equal(t, "Alice", got[1].Author)
	assert.Equal(t, "Carol", got[1].Narrator)
	assert.ElementsMatch(t, []string{"Fantasy", "Mystery"}, got[1].Genres)

	// Multi-role contributor on Book 3.
	assert.Equal(t, "Bob", got[2].Author)
	assert.Equal(t, "Bob", got[2].Narrator)

	// === No-N+1 property: each underlying method called exactly once. ===
	assert.Equal(t, 1, store.getContributorsByIDsCalls,
		"contributors should be fetched once for the whole batch")
	assert.Equal(t, 1, store.getSeriesByIDsCalls,
		"series should be fetched once for the whole batch")
	assert.Equal(t, 1, store.getGenresByIDsCalls,
		"genres should be fetched once for the whole batch")
	assert.Equal(t, 1, store.getTagsForBookIDsCalls,
		"tags should be fetched once for the whole batch")
	assert.Zero(t, store.getTagsForBookCalls,
		"per-book GetTagsForBook should not be used in batch path")
	assert.Zero(t, store.getSeriesCalls,
		"per-id GetSeries should not be used in batch path")

	// IDs were de-duplicated in the batch fetch.
	assert.ElementsMatch(t, []string{"c1", "c2", "c3"}, store.lastContributorIDs)
	assert.ElementsMatch(t, []string{"s1"}, store.lastSeriesIDs)
	assert.ElementsMatch(t, []string{"g1", "g2"}, store.lastGenreIDs)
	assert.ElementsMatch(t, []string{"b1", "b2", "b3"}, store.lastTagBookIDs)
}

func TestEnrichBooks_MissingReferencesAreGraceful(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	// Only c1 and s1 exist; c-missing and s-missing will be absent.
	store.contributors["c1"] = &domain.Contributor{Syncable: domain.Syncable{ID: "c1"}, Name: "Alice"}
	store.series["s1"] = &domain.Series{Syncable: domain.Syncable{ID: "s1"}, Name: "Real Series"}

	books := []*domain.Book{
		makeBook("b1", "Book 1",
			[]domain.BookContributor{
				{ContributorID: "c1", Roles: []domain.ContributorRole{domain.RoleAuthor}},
				{ContributorID: "c-missing", Roles: []domain.ContributorRole{domain.RoleNarrator}},
			},
			[]domain.BookSeries{
				{SeriesID: "s1", Sequence: "1"},
				{SeriesID: "s-missing", Sequence: "2"},
			},
			nil,
		),
		makeBook("b2", "Book 2", nil, nil, nil),
	}

	e := NewEnricher(store)
	got, err := e.EnrichBooks(context.Background(), books)
	require.NoError(t, err)
	require.Len(t, got, 2)

	assert.Equal(t, "Alice", got[0].Author)
	assert.Empty(t, got[0].Narrator, "missing contributor leaves narrator empty")
	require.Len(t, got[0].Contributors, 2)
	assert.Empty(t, got[0].Contributors[1].Name)

	require.Len(t, got[0].SeriesInfo, 1, "missing series is dropped")
	assert.Equal(t, "Real Series", got[0].SeriesInfo[0].Name)

	// Book with nothing.
	assert.Empty(t, got[1].Contributors)
	assert.Empty(t, got[1].SeriesInfo)
}

func TestEnrichBooks_ContributorErrorPropagates(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.contributorsErr = errors.New("db down")
	e := NewEnricher(store)

	books := []*domain.Book{
		makeBook("b1", "Book", []domain.BookContributor{
			{ContributorID: "c1", Roles: []domain.ContributorRole{domain.RoleAuthor}},
		}, nil, nil),
	}

	_, err := e.EnrichBooks(context.Background(), books)
	require.Error(t, err)
}

func TestEnrichBooks_GenreAndTagErrorsAreNonFatal(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.contributors["c1"] = &domain.Contributor{Syncable: domain.Syncable{ID: "c1"}, Name: "Alice"}
	store.genresErr = errors.New("genre lookup failed")
	store.tagsErr = errors.New("tag lookup failed")

	books := []*domain.Book{
		makeBook("b1", "Book 1",
			[]domain.BookContributor{
				{ContributorID: "c1", Roles: []domain.ContributorRole{domain.RoleAuthor}},
			},
			nil,
			[]string{"g1"},
		),
	}

	e := NewEnricher(store)
	got, err := e.EnrichBooks(context.Background(), books)
	require.NoError(t, err, "genre/tag failures must not fail the whole batch")
	require.Len(t, got, 1)
	assert.Equal(t, "Alice", got[0].Author)
	assert.Empty(t, got[0].Genres, "genre lookup failure leaves genres empty")
	assert.Empty(t, got[0].Tags, "tag lookup failure leaves tags empty")
}
