package search

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestIndex creates a temporary search index for testing.
func setupTestIndex(t *testing.T) (*SearchIndex, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "search-test-*")
	require.NoError(t, err)

	index, err := NewSearchIndex(Options{
		DataPath: tmpDir,
		Logger:   nil,
	})
	require.NoError(t, err)

	cleanup := func() {
		_ = index.Close()
		_ = os.RemoveAll(tmpDir)
	}

	return index, cleanup
}

func TestNewSearchIndex(t *testing.T) {
	index, cleanup := setupTestIndex(t)
	defer cleanup()

	count, err := index.DocumentCount()
	require.NoError(t, err)
	assert.Equal(t, uint64(0), count)
}

func TestSearchIndex_IndexDocument(t *testing.T) {
	index, cleanup := setupTestIndex(t)
	defer cleanup()

	doc := &SearchDocument{
		ID:     "book-123",
		Type:   DocTypeBook,
		Name:   "The Hobbit",
		Author: "J.R.R. Tolkien",
	}

	err := index.IndexDocument(doc)
	require.NoError(t, err)

	count, err := index.DocumentCount()
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)
}

func TestSearchIndex_IndexDocuments_Batch(t *testing.T) {
	index, cleanup := setupTestIndex(t)
	defer cleanup()

	docs := []*SearchDocument{
		{ID: "book-1", Type: DocTypeBook, Name: "Book One"},
		{ID: "book-2", Type: DocTypeBook, Name: "Book Two"},
		{ID: "book-3", Type: DocTypeBook, Name: "Book Three"},
	}

	err := index.IndexDocuments(docs)
	require.NoError(t, err)

	count, err := index.DocumentCount()
	require.NoError(t, err)
	assert.Equal(t, uint64(3), count)
}

func TestSearchIndex_DeleteDocument(t *testing.T) {
	index, cleanup := setupTestIndex(t)
	defer cleanup()

	doc := &SearchDocument{
		ID:   "book-123",
		Type: DocTypeBook,
		Name: "Test Book",
	}

	err := index.IndexDocument(doc)
	require.NoError(t, err)

	err = index.DeleteDocument("book-123")
	require.NoError(t, err)

	count, err := index.DocumentCount()
	require.NoError(t, err)
	assert.Equal(t, uint64(0), count)
}

func TestSearchIndex_Search_Basic(t *testing.T) {
	index, cleanup := setupTestIndex(t)
	defer cleanup()

	// Index some test documents
	docs := []*SearchDocument{
		{ID: "book-1", Type: DocTypeBook, Name: "The Hobbit", Author: "J.R.R. Tolkien"},
		{ID: "book-2", Type: DocTypeBook, Name: "The Lord of the Rings", Author: "J.R.R. Tolkien"},
		{ID: "book-3", Type: DocTypeBook, Name: "Harry Potter", Author: "J.K. Rowling"},
	}

	err := index.IndexDocuments(docs)
	require.NoError(t, err)

	ctx := context.Background()

	// Search for "Tolkien"
	result, err := index.Search(ctx, SearchParams{
		Query: "Tolkien",
		Limit: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, uint64(2), result.Total)
	assert.Len(t, result.Hits, 2)
}

func TestSearchIndex_Search_ByType(t *testing.T) {
	index, cleanup := setupTestIndex(t)
	defer cleanup()

	docs := []*SearchDocument{
		{ID: "book-1", Type: DocTypeBook, Name: "The Hobbit"},
		{ID: "contrib-1", Type: DocTypeContributor, Name: "Tolkien"},
		{ID: "series-1", Type: DocTypeSeries, Name: "Middle Earth"},
	}

	err := index.IndexDocuments(docs)
	require.NoError(t, err)

	ctx := context.Background()

	// Search for books only
	result, err := index.Search(ctx, SearchParams{
		Query: "",
		Types: []string{string(DocTypeBook)},
		Limit: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, uint64(1), result.Total)
	assert.Equal(t, "book-1", result.Hits[0].ID)
}

func TestSearchIndex_Search_Prefix(t *testing.T) {
	index, cleanup := setupTestIndex(t)
	defer cleanup()

	doc := &SearchDocument{
		ID:   "book-1",
		Type: DocTypeBook,
		Name: "The Hobbit",
	}

	err := index.IndexDocument(doc)
	require.NoError(t, err)

	ctx := context.Background()

	// Search with prefix - should find result
	result, err := index.Search(ctx, SearchParams{
		Query: "Hobb", // Prefix of Hobbit
		Limit: 10,
	})
	require.NoError(t, err)
	// Prefix search should find the result
	assert.GreaterOrEqual(t, result.Total, uint64(1))
}

func TestSearchIndex_Search_GenrePath(t *testing.T) {
	index, cleanup := setupTestIndex(t)
	defer cleanup()

	docs := []*SearchDocument{
		{
			ID:         "book-1",
			Type:       DocTypeBook,
			Name:       "Epic Fantasy Book",
			GenrePaths: []string{"/fiction/fantasy/epic-fantasy", "/fiction/fantasy", "/fiction"},
		},
		{
			ID:         "book-2",
			Type:       DocTypeBook,
			Name:       "Romance Book",
			GenrePaths: []string{"/fiction/romance", "/fiction"},
		},
	}

	err := index.IndexDocuments(docs)
	require.NoError(t, err)

	ctx := context.Background()

	// Search for fantasy genre path - should find the epic fantasy book
	result, err := index.Search(ctx, SearchParams{
		Query:     "",
		GenrePath: "/fiction/fantasy",
		Limit:     10,
	})
	require.NoError(t, err)
	assert.Equal(t, uint64(1), result.Total)
	assert.Equal(t, "book-1", result.Hits[0].ID)

	// Search for all fiction - should find both
	result, err = index.Search(ctx, SearchParams{
		Query:     "",
		GenrePath: "/fiction",
		Limit:     10,
	})
	require.NoError(t, err)
	assert.Equal(t, uint64(2), result.Total)
}

func TestSearchIndex_Search_Duration(t *testing.T) {
	index, cleanup := setupTestIndex(t)
	defer cleanup()

	docs := []*SearchDocument{
		{ID: "book-1", Type: DocTypeBook, Name: "Short Book", Duration: 3600},   // 1 hour
		{ID: "book-2", Type: DocTypeBook, Name: "Medium Book", Duration: 36000}, // 10 hours
		{ID: "book-3", Type: DocTypeBook, Name: "Long Book", Duration: 72000},   // 20 hours
	}

	err := index.IndexDocuments(docs)
	require.NoError(t, err)

	ctx := context.Background()

	// Filter by duration range
	result, err := index.Search(ctx, SearchParams{
		Query:       "",
		MinDuration: 10000,
		MaxDuration: 50000,
		Limit:       10,
	})
	require.NoError(t, err)
	assert.Equal(t, uint64(1), result.Total)
	assert.Equal(t, "book-2", result.Hits[0].ID)
}

func TestSearchIndex_Rebuild(t *testing.T) {
	index, cleanup := setupTestIndex(t)
	defer cleanup()

	// Index a document
	doc := &SearchDocument{ID: "book-1", Type: DocTypeBook, Name: "Test"}
	err := index.IndexDocument(doc)
	require.NoError(t, err)

	// Verify it exists
	count, err := index.DocumentCount()
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)

	// Rebuild - should clear the index
	err = index.Rebuild()
	require.NoError(t, err)

	// Verify it's empty
	count, err = index.DocumentCount()
	require.NoError(t, err)
	assert.Equal(t, uint64(0), count)
}

func TestSearchIndex_Persistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "search-persist-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create index and add document
	index1, err := NewSearchIndex(Options{DataPath: tmpDir})
	require.NoError(t, err)

	doc := &SearchDocument{ID: "book-1", Type: DocTypeBook, Name: "Test Book"}
	err = index1.IndexDocument(doc)
	require.NoError(t, err)

	err = index1.Close()
	require.NoError(t, err)

	// Reopen index and verify document is still there
	index2, err := NewSearchIndex(Options{DataPath: tmpDir})
	require.NoError(t, err)
	defer index2.Close()

	count, err := index2.DocumentCount()
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)

	// Verify we can search for it
	ctx := context.Background()
	result, err := index2.Search(ctx, SearchParams{Query: "Test", Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, uint64(1), result.Total)
}

func TestBookToSearchDocument(t *testing.T) {
	book := &domain.Book{
		Syncable: domain.Syncable{
			ID: "book-123",
		},
		Title:         "The Great Book",
		Subtitle:      "A Story",
		Description:   "A wonderful tale",
		TotalDuration: 7200,
		PublishYear:   "2023",
		SeriesID:      "series-456",
		Sequence:      "1",
	}

	doc := BookToSearchDocument(
		book,
		"Jane Author",
		"John Narrator",
		"Great Series",
		[]string{"/fiction/fantasy"},
		[]string{"fantasy"},
	)

	assert.Equal(t, "book-123", doc.ID)
	assert.Equal(t, DocTypeBook, doc.Type)
	assert.Equal(t, "The Great Book", doc.Name)
	assert.Equal(t, "A Story", doc.Subtitle)
	assert.Equal(t, "Jane Author", doc.Author)
	assert.Equal(t, "John Narrator", doc.Narrator)
	assert.Equal(t, "Great Series", doc.SeriesName)
	assert.Equal(t, int64(7200), doc.Duration)
	assert.Equal(t, 2023, doc.PublishYear)
	assert.Equal(t, []string{"/fiction/fantasy"}, doc.GenrePaths)
	assert.Equal(t, []string{"fantasy"}, doc.GenreSlugs)
}

func TestContributorToSearchDocument(t *testing.T) {
	contributor := &domain.Contributor{
		Syncable: domain.Syncable{
			ID: "contrib-123",
		},
		Name: "Famous Author",
	}

	doc := ContributorToSearchDocument(contributor, 42)

	assert.Equal(t, "contrib-123", doc.ID)
	assert.Equal(t, DocTypeContributor, doc.Type)
	assert.Equal(t, "Famous Author", doc.Name)
	assert.Equal(t, 42, doc.BookCount)
}

func TestSeriesToSearchDocument(t *testing.T) {
	series := &domain.Series{
		Syncable: domain.Syncable{
			ID: "series-123",
		},
		Name:        "Epic Series",
		Description: "An epic tale spanning books",
		TotalBooks:  5,
	}

	doc := SeriesToSearchDocument(series)

	assert.Equal(t, "series-123", doc.ID)
	assert.Equal(t, DocTypeSeries, doc.Type)
	assert.Equal(t, "Epic Series", doc.Name)
	assert.Equal(t, "An epic tale spanning books", doc.Description)
	assert.Equal(t, 5, doc.BookCount)
}


func TestSearchIndex_LargeBatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large batch test in short mode")
	}

	index, cleanup := setupTestIndex(t)
	defer cleanup()

	// Create 1000 documents to test chunking (batch size is 500)
	docs := make([]*SearchDocument, 1000)
	for i := 0; i < 1000; i++ {
		docs[i] = &SearchDocument{
			ID:   "book-" + string(rune('0'+i%10)) + string(rune('0'+i/10%10)) + string(rune('0'+i/100%10)),
			Type: DocTypeBook,
			Name: "Book Number " + string(rune('0'+i%10)),
		}
	}

	start := time.Now()
	err := index.IndexDocuments(docs)
	require.NoError(t, err)
	t.Logf("Indexed 1000 documents in %v", time.Since(start))

	count, err := index.DocumentCount()
	require.NoError(t, err)
	assert.Equal(t, uint64(1000), count)
}

func TestSearchParams_Defaults(t *testing.T) {
	params := SearchParams{}

	// Empty params should have sensible behavior when used
	assert.Equal(t, "", params.Query)
	assert.Nil(t, params.Types)
	assert.Equal(t, 0, params.Limit) // Caller should set default
}
