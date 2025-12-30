package search

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/blevesearch/bleve/v2"
)

// SearchIndex wraps a Bleve index with domain-specific operations.
//
// Thread safety: All public methods are safe for concurrent use.
// The mutex protects against index corruption during rebuild operations.
type SearchIndex struct {
	index  bleve.Index
	path   string
	logger *slog.Logger
	mu     sync.RWMutex // Protects index operations during rebuild
}

// Options configures the search index.
type Options struct {
	DataPath string       // Directory for index storage
	Logger   *slog.Logger // Logger for operations (uses discard if nil)
}

// mappingVersion is incremented whenever the index mapping changes.
// This triggers an automatic rebuild on startup when the version doesn't match.
const mappingVersion = "4"

// NewSearchIndex creates or opens a search index.
// If an existing index is found, it opens it. Otherwise, creates a new one.
// If the existing index is corrupted or has an outdated mapping, it's removed and recreated.
func NewSearchIndex(opts Options) (*SearchIndex, error) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}

	indexPath := filepath.Join(opts.DataPath, "search.bleve")
	versionPath := filepath.Join(opts.DataPath, "search.version")

	var index bleve.Index
	var err error
	needsRebuild := false

	// Check mapping version - rebuild if version file missing or mismatched
	indexExists := false
	if _, statErr := os.Stat(indexPath); statErr == nil {
		indexExists = true
	}

	if indexExists {
		existingVersion, readErr := os.ReadFile(versionPath)
		if readErr != nil {
			// Version file missing but index exists - this is an old index
			logger.Info("search index has no version file, will rebuild with current mapping",
				"new_version", mappingVersion,
			)
			needsRebuild = true
		} else if string(existingVersion) != mappingVersion {
			logger.Info("search index mapping version changed, will rebuild",
				"old_version", string(existingVersion),
				"new_version", mappingVersion,
			)
			needsRebuild = true
		}
	}

	// Try to open existing index (if not forcing rebuild)
	if !needsRebuild && indexExists {
		index, err = bleve.Open(indexPath)
		if err != nil {
			logger.Warn("failed to open existing index, will recreate",
				"path", indexPath,
				"error", err,
			)
			needsRebuild = true
		}
	}

	// Remove old index if rebuild needed
	if needsRebuild {
		if removeErr := os.RemoveAll(indexPath); removeErr != nil {
			return nil, fmt.Errorf("remove old index: %w", removeErr)
		}
		index = nil
	}

	// Create new index if needed
	if index == nil {
		indexMapping := buildIndexMapping()
		index, err = bleve.New(indexPath, indexMapping)
		if err != nil {
			return nil, fmt.Errorf("create index: %w", err)
		}
		// Write version file
		if writeErr := os.WriteFile(versionPath, []byte(mappingVersion), 0644); writeErr != nil {
			logger.Warn("failed to write search version file", "error", writeErr)
		}
		logger.Info("created new search index", "path", indexPath, "mapping_version", mappingVersion)
	} else {
		logger.Info("opened existing search index", "path", indexPath)
	}

	return &SearchIndex{
		index:  index,
		path:   indexPath,
		logger: logger,
	}, nil
}

// Close closes the index and releases resources.
func (s *SearchIndex) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.index.Close()
}

// IndexDocument indexes a single document.
func (s *SearchIndex) IndexDocument(doc *SearchDocument) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Convert to map to ensure field names match the mapping (lowercase)
	return s.index.Index(doc.ID, doc.ToMap())
}

// IndexDocuments indexes multiple documents in a batch.
// This is significantly faster than calling IndexDocument in a loop.
// For large document sets (>500), documents are processed in chunks
// to prevent memory pressure during initial indexing.
func (s *SearchIndex) IndexDocuments(docs []*SearchDocument) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	const batchSize = 500

	for i := 0; i < len(docs); i += batchSize {
		end := i + batchSize
		if end > len(docs) {
			end = len(docs)
		}
		chunk := docs[i:end]

		batch := s.index.NewBatch()
		for _, doc := range chunk {
			// Convert to map to ensure field names match the mapping (lowercase)
			if err := batch.Index(doc.ID, doc.ToMap()); err != nil {
				return fmt.Errorf("batch index %s: %w", doc.ID, err)
			}
		}

		if err := s.index.Batch(batch); err != nil {
			return fmt.Errorf("commit batch %d-%d: %w", i, end, err)
		}
	}

	return nil
}

// DeleteDocument removes a document from the index.
func (s *SearchIndex) DeleteDocument(id string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.index.Delete(id)
}

// DeleteDocuments removes multiple documents from the index.
func (s *SearchIndex) DeleteDocuments(ids []string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	batch := s.index.NewBatch()
	for _, id := range ids {
		batch.Delete(id)
	}

	return s.index.Batch(batch)
}

// DocumentCount returns the total number of indexed documents.
func (s *SearchIndex) DocumentCount() (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.index.DocCount()
}

// Rebuild drops the existing index and creates a new one.
// Used for full reindex operations when schema changes or corruption occurs.
//
// IMPORTANT: This acquires an exclusive lock and blocks all other operations.
// Callers should ensure this is called during maintenance windows or
// handle the temporary unavailability gracefully.
func (s *SearchIndex) Rebuild() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Close existing index
	if err := s.index.Close(); err != nil {
		return fmt.Errorf("close index: %w", err)
	}

	// Remove index directory
	if err := os.RemoveAll(s.path); err != nil {
		return fmt.Errorf("remove index: %w", err)
	}

	// Create fresh index
	indexMapping := buildIndexMapping()
	index, err := bleve.New(s.path, indexMapping)
	if err != nil {
		return fmt.Errorf("create index: %w", err)
	}

	s.index = index
	s.logger.Info("rebuilt search index", "path", s.path)

	return nil
}
