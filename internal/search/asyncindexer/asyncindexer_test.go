package asyncindexer

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
)

// noopSearchIndexer is a no-op implementation of store.SearchIndexer for tests.
type noopSearchIndexer struct{}

func (noopSearchIndexer) IndexBook(_ context.Context, _ *domain.Book) error               { return nil }
func (noopSearchIndexer) DeleteBook(_ context.Context, _ string) error                    { return nil }
func (noopSearchIndexer) IndexContributor(_ context.Context, _ *domain.Contributor) error { return nil }
func (noopSearchIndexer) DeleteContributor(_ context.Context, _ string) error             { return nil }
func (noopSearchIndexer) IndexSeries(_ context.Context, _ *domain.Series) error           { return nil }
func (noopSearchIndexer) DeleteSeries(_ context.Context, _ string) error                  { return nil }

func TestIndexer_LastTickAdvances(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	idx := New(&noopSearchIndexer{}, logger)
	idx.Start(context.Background())
	t.Cleanup(func() { _ = idx.Shutdown(context.Background()) })

	before := idx.LastTick()
	idx.SubmitIndexBook(&domain.Book{Syncable: domain.Syncable{ID: "lt-1"}})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if idx.LastTick().After(before) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !idx.LastTick().After(before) {
		t.Fatalf("LastTick did not advance: before=%v after=%v", before, idx.LastTick())
	}
}

func TestIndexer_QueueDepthZero(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	idx := New(&noopSearchIndexer{}, logger)
	if got := idx.QueueDepth(); got != 0 {
		t.Fatalf("fresh indexer queue depth = %d, want 0", got)
	}
}

func TestIndexer_DropsCounted(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	release := make(chan struct{})
	idx := New(&blockingIndexer{block: release}, logger)
	idx.Start(context.Background())
	t.Cleanup(func() {
		close(release)
		_ = idx.Shutdown(context.Background())
	})

	// Saturate the queue. The queue capacity is queueDepth (1024); the worker
	// grabs one job and blocks, leaving 1023 slots. Submit well above the
	// capacity ceiling so some jobs are guaranteed to be dropped.
	const submit = 2048
	for range submit {
		idx.SubmitIndexBook(&domain.Book{Syncable: domain.Syncable{ID: "spam"}})
	}

	if got := idx.Drops(); got <= 0 {
		t.Fatalf("expected at least one drop after %d submissions, got %d; queue capacity is %d", submit, got, queueDepth)
	}
}

// blockingIndexer blocks IndexBook until release is closed.
type blockingIndexer struct {
	noopSearchIndexer
	block chan struct{}
}

func (b *blockingIndexer) IndexBook(_ context.Context, _ *domain.Book) error {
	<-b.block
	return nil
}
