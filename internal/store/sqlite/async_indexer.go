package sqlite

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// indexQueueDepth is the buffered channel capacity for pending index jobs.
// Submit drops jobs (with a logged warning) when the queue is full, since
// search is best-effort and a full reindex can recover any drops.
const indexQueueDepth = 1024

// indexWorkers is the number of concurrent worker goroutines draining the
// index job queue. Bleve is single-writer, so we keep this at 1 by default.
const indexWorkers = 1

// indexJobOp identifies which SearchIndexer method a job should invoke.
type indexJobOp int

const (
	opIndexBook indexJobOp = iota
	opDeleteBook
	opIndexContributor
	opDeleteContributor
	opIndexSeries
	opDeleteSeries
)

// indexJob is a sum-type-style envelope for any operation we can submit to
// the search indexer. Only the field corresponding to op is populated.
type indexJob struct {
	op          indexJobOp
	id          string
	book        *domain.Book
	contributor *domain.Contributor
	series      *domain.Series
}

// asyncIndexer runs SearchIndexer operations off the request path on a
// dedicated worker goroutine, with bounded queueing and lifecycle
// management decoupled from any caller's context.
type asyncIndexer struct {
	indexer store.SearchIndexer
	logger  *slog.Logger

	jobs chan indexJob

	ctx    context.Context //nolint:containedctx // worker lifecycle is owned by the indexer
	cancel context.CancelFunc

	wg sync.WaitGroup

	startOnce sync.Once
	stopOnce  sync.Once
}

// newAsyncIndexer constructs an asyncIndexer wrapping the given SearchIndexer.
// Call Start to launch its workers and Shutdown to drain and stop.
func newAsyncIndexer(indexer store.SearchIndexer, logger *slog.Logger) *asyncIndexer {
	return &asyncIndexer{
		indexer: indexer,
		logger:  logger,
		jobs:    make(chan indexJob, indexQueueDepth),
	}
}

// Start launches the worker goroutines. It is safe to call multiple times;
// only the first call has effect. The provided ctx is used as the parent
// for the indexer's own cancelable context — when ctx is canceled, workers
// finish their current job and exit.
func (a *asyncIndexer) Start(ctx context.Context) {
	a.startOnce.Do(func() {
		a.ctx, a.cancel = context.WithCancel(ctx)
		for range indexWorkers {
			a.wg.Add(1)
			go a.run()
		}
	})
}

// Submit enqueues a job for asynchronous processing. It never blocks: when
// the queue is full, the job is dropped and a warning is logged. Search is
// best-effort and may be reconciled by a full reindex.
func (a *asyncIndexer) Submit(job indexJob) {
	select {
	case a.jobs <- job:
	default:
		a.logger.Warn("search index queue full, dropping job",
			"op", job.op,
			"id", a.jobIdentifier(job),
			"queue_depth", indexQueueDepth,
		)
	}
}

// Shutdown stops accepting new work, drains in-flight jobs, and waits for
// workers to exit. If ctx is canceled or expires before workers finish, it
// returns ctx.Err.
func (a *asyncIndexer) Shutdown(ctx context.Context) error {
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
func (a *asyncIndexer) run() {
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
func (a *asyncIndexer) execute(job indexJob) {
	// Bound the per-job runtime independently of the indexer's lifecycle so
	// a stuck bleve write cannot wedge the worker indefinitely.
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()

	var err error
	switch job.op {
	case opIndexBook:
		if job.book != nil {
			err = a.indexer.IndexBook(ctx, job.book)
		}
	case opDeleteBook:
		err = a.indexer.DeleteBook(ctx, job.id)
	case opIndexContributor:
		if job.contributor != nil {
			err = a.indexer.IndexContributor(ctx, job.contributor)
		}
	case opDeleteContributor:
		err = a.indexer.DeleteContributor(ctx, job.id)
	case opIndexSeries:
		if job.series != nil {
			err = a.indexer.IndexSeries(ctx, job.series)
		}
	case opDeleteSeries:
		err = a.indexer.DeleteSeries(ctx, job.id)
	default:
		a.logger.Warn("unknown index job op", "op", job.op)
		return
	}

	if err != nil && !errors.Is(err, context.Canceled) {
		a.logger.Warn("search index operation failed",
			"op", job.op,
			"id", a.jobIdentifier(job),
			"error", err,
		)
	}
}

// jobIdentifier returns a best-effort identifier for logging.
func (a *asyncIndexer) jobIdentifier(job indexJob) string {
	switch {
	case job.book != nil:
		return job.book.ID
	case job.contributor != nil:
		return job.contributor.ID
	case job.series != nil:
		return job.series.ID
	default:
		return job.id
	}
}
