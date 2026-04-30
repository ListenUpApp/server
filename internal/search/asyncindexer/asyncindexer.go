// Package asyncindexer provides a bounded, asynchronous queue for search-index
// operations. It wraps a store.SearchIndexer and dispatches work to a fixed
// pool of worker goroutines, dropping jobs when the queue is full (search is
// best-effort; a full reindex can recover any drops).
package asyncindexer

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// queueDepth is the buffered channel capacity for pending index jobs.
// Submit drops jobs (with a logged warning) when the queue is full.
const queueDepth = 1024

// workers is the number of concurrent worker goroutines draining the queue.
// Bleve is single-writer, so we keep this at 1 by default.
const workers = 1

// Op identifies which SearchIndexer method a Job should invoke.
type Op int

// Op constants enumerate the operations the Indexer can execute.
const (
	OpIndexBook Op = iota
	OpDeleteBook
	OpIndexContributor
	OpDeleteContributor
	OpIndexSeries
	OpDeleteSeries
)

// Job is a sum-type-style envelope for any operation submitted to the indexer.
// Only the field corresponding to Op is populated.
type Job struct {
	Op          Op
	ID          string
	Book        *domain.Book
	Contributor *domain.Contributor
	Series      *domain.Series
}

// Indexer runs SearchIndexer operations off the request path on a dedicated
// worker goroutine, with bounded queueing and lifecycle management decoupled
// from any caller's context.
type Indexer struct {
	indexer store.SearchIndexer
	logger  *slog.Logger

	jobs chan Job

	ctx    context.Context //nolint:containedctx // worker lifecycle is owned by the Indexer
	cancel context.CancelFunc

	wg sync.WaitGroup

	startOnce sync.Once
	stopOnce  sync.Once
}

// New constructs an Indexer wrapping the given SearchIndexer.
// Call Start to launch its workers and Shutdown to drain and stop.
func New(indexer store.SearchIndexer, logger *slog.Logger) *Indexer {
	return &Indexer{
		indexer: indexer,
		logger:  logger,
		jobs:    make(chan Job, queueDepth),
	}
}

// Start launches the worker goroutines. It is safe to call multiple times;
// only the first call has effect. The provided ctx is used as the parent
// for the indexer's own cancelable context — when ctx is canceled, workers
// finish their current job and exit.
func (a *Indexer) Start(ctx context.Context) {
	a.startOnce.Do(func() {
		a.ctx, a.cancel = context.WithCancel(ctx)
		for range workers {
			a.wg.Add(1)
			go a.run()
		}
	})
}

// Submit enqueues a job for asynchronous processing. It never blocks: when
// the queue is full, the job is dropped and a warning is logged.
func (a *Indexer) Submit(job Job) {
	select {
	case a.jobs <- job:
	default:
		a.logger.Warn("search index queue full, dropping job",
			"op", job.Op,
			"id", a.jobIdentifier(job),
			"queue_depth", queueDepth,
		)
	}
}

// SubmitIndexBook enqueues a non-blocking search index update for a book.
func (a *Indexer) SubmitIndexBook(book *domain.Book) {
	a.Submit(Job{Op: OpIndexBook, Book: book})
}

// SubmitDeleteBook enqueues a non-blocking search index removal for a book.
func (a *Indexer) SubmitDeleteBook(id string) {
	a.Submit(Job{Op: OpDeleteBook, ID: id})
}

// SubmitIndexContributor enqueues a non-blocking search index update for a contributor.
func (a *Indexer) SubmitIndexContributor(c *domain.Contributor) {
	a.Submit(Job{Op: OpIndexContributor, Contributor: c})
}

// SubmitDeleteContributor enqueues a non-blocking search index removal for a contributor.
func (a *Indexer) SubmitDeleteContributor(id string) {
	a.Submit(Job{Op: OpDeleteContributor, ID: id})
}

// SubmitIndexSeries enqueues a non-blocking search index update for a series.
func (a *Indexer) SubmitIndexSeries(s *domain.Series) {
	a.Submit(Job{Op: OpIndexSeries, Series: s})
}

// SubmitDeleteSeries enqueues a non-blocking search index removal for a series.
func (a *Indexer) SubmitDeleteSeries(id string) {
	a.Submit(Job{Op: OpDeleteSeries, ID: id})
}

// Shutdown stops accepting new work, drains in-flight jobs, and waits for
// workers to exit. If ctx is canceled or expires before workers finish,
// it returns ctx.Err.
func (a *Indexer) Shutdown(ctx context.Context) error {
	var err error
	a.stopOnce.Do(func() {
		// Signal workers to stop dequeueing once the channel is drained.
		close(a.jobs)

		done := make(chan struct{})
		go func() {
			a.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Clean drain.
		case <-ctx.Done():
			// Caller's grace period expired; cancel workers mid-flight.
			if a.cancel != nil {
				a.cancel()
			}
			err = ctx.Err()
		}

		// Always cancel the worker context so any leftover goroutines
		// using a.ctx see Done.
		if a.cancel != nil {
			a.cancel()
		}
	})
	return err
}

// run is the worker loop. It exits when the jobs channel is closed and
// drained, or when the indexer's context is canceled.
func (a *Indexer) run() {
	defer a.wg.Done()
	for {
		select {
		case <-a.ctx.Done():
			return
		case job, ok := <-a.jobs:
			if !ok {
				return
			}
			a.execute(job)
		}
	}
}

// execute dispatches a single job to the underlying SearchIndexer using the
// indexer's own context, so request-scoped cancellation cannot abort it.
func (a *Indexer) execute(job Job) {
	// Bound the per-job runtime independently of the indexer's lifecycle so
	// a stuck bleve write cannot wedge the worker indefinitely.
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()

	var err error
	switch job.Op {
	case OpIndexBook:
		if job.Book != nil {
			err = a.indexer.IndexBook(ctx, job.Book)
		}
	case OpDeleteBook:
		err = a.indexer.DeleteBook(ctx, job.ID)
	case OpIndexContributor:
		if job.Contributor != nil {
			err = a.indexer.IndexContributor(ctx, job.Contributor)
		}
	case OpDeleteContributor:
		err = a.indexer.DeleteContributor(ctx, job.ID)
	case OpIndexSeries:
		if job.Series != nil {
			err = a.indexer.IndexSeries(ctx, job.Series)
		}
	case OpDeleteSeries:
		err = a.indexer.DeleteSeries(ctx, job.ID)
	default:
		a.logger.Warn("unknown index job op", "op", job.Op)
		return
	}

	if err != nil && !errors.Is(err, context.Canceled) {
		a.logger.Warn("search index operation failed",
			"op", job.Op,
			"id", a.jobIdentifier(job),
			"error", err,
		)
	}
}

// jobIdentifier returns a best-effort identifier for logging.
func (a *Indexer) jobIdentifier(job Job) string {
	switch {
	case job.Book != nil:
		return job.Book.ID
	case job.Contributor != nil:
		return job.Contributor.ID
	case job.Series != nil:
		return job.Series.ID
	default:
		return job.ID
	}
}
