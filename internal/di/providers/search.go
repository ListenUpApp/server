package providers

import (
	"context"
	"time"

	"github.com/samber/do/v2"

	"github.com/listenupapp/listenup-server/internal/config"
	"github.com/listenupapp/listenup-server/internal/logger"
	"github.com/listenupapp/listenup-server/internal/search"
	"github.com/listenupapp/listenup-server/internal/search/asyncindexer"
	"github.com/listenupapp/listenup-server/internal/service"
)

// SearchIndexHandle wraps the search index with shutdown capability.
type SearchIndexHandle struct {
	*search.SearchIndex
}

// Shutdown implements do.Shutdownable.
func (h *SearchIndexHandle) Shutdown() error {
	return h.Close()
}

// ProvideSearchIndex provides the Bleve search index.
func ProvideSearchIndex(i do.Injector) (*SearchIndexHandle, error) {
	cfg := do.MustInvoke[*config.Config](i)
	log := do.MustInvoke[*logger.Logger](i)

	index, err := search.NewSearchIndex(search.Options{
		DataPath: cfg.Metadata.BasePath,
		Logger:   log.Logger,
	})
	if err != nil {
		return nil, err
	}

	docCount, _ := index.DocumentCount()
	log.Info("Search index initialized", "documents", docCount)

	return &SearchIndexHandle{SearchIndex: index}, nil
}

// AsyncIndexerHandle wraps the async indexer with shutdown capability.
type AsyncIndexerHandle struct {
	*asyncindexer.Indexer
}

// Shutdown implements do.Shutdownable.
func (h *AsyncIndexerHandle) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return h.Indexer.Shutdown(ctx)
}

// ProvideAsyncIndexer provides the async search indexer.
// It wraps the SearchService (which implements store.SearchIndexer) and starts
// the worker goroutine. Shutdown drains in-flight jobs with a grace period.
func ProvideAsyncIndexer(i do.Injector) (*AsyncIndexerHandle, error) {
	searchSvc := do.MustInvoke[*service.SearchService](i)
	log := do.MustInvoke[*logger.Logger](i)

	idx := asyncindexer.New(searchSvc, log.Logger)
	idx.Start(context.Background())

	return &AsyncIndexerHandle{Indexer: idx}, nil
}

// ProvideSearchService provides the search service.
func ProvideSearchService(i do.Injector) (*service.SearchService, error) {
	indexHandle := do.MustInvoke[*SearchIndexHandle](i)
	storeHandle := do.MustInvoke[*StoreHandle](i)
	log := do.MustInvoke[*logger.Logger](i)

	return service.NewSearchService(indexHandle.SearchIndex, storeHandle.Store, log.Logger), nil
}

// TriggerSearchReindexIfNeeded checks if reindexing is needed and triggers it.
// Should be called after all services are wired.
func TriggerSearchReindexIfNeeded(i do.Injector) {
	searchService := do.MustInvoke[*service.SearchService](i)
	storeHandle := do.MustInvoke[*StoreHandle](i)
	log := do.MustInvoke[*logger.Logger](i)

	docCount, _ := searchService.DocumentCount()
	if docCount > 0 {
		return
	}

	// Check if we have books that need indexing
	ctx := context.Background()
	books, err := storeHandle.ListAllBooks(ctx)
	if err != nil || len(books) == 0 {
		return
	}

	log.Info("Search index is empty but books exist, triggering initial reindex",
		"book_count", len(books),
	)

	go func() {
		reindexCtx := context.Background()
		if err := searchService.ReindexAll(reindexCtx); err != nil {
			log.Error("Initial search reindex failed", "error", err)
		} else {
			count, _ := searchService.DocumentCount()
			log.Info("Initial search reindex completed", "documents", count)
		}
	}()
}
