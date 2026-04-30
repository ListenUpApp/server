package api

import (
	"context"
	"errors"
	"expvar"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// importJobManager owns the lifecycle of background ABS import analysis
// goroutines. It mirrors the cancelable-worker pattern used by the file
// watcher and cleanup jobs in internal/di/providers/workers.go: a
// context+cancel pair tracks shutdown intent (the context is captured in the
// per-job goroutine closure rather than stored on the struct, to avoid the
// containedctx anti-pattern) and a sync.WaitGroup tracks active workers so
// shutdown can wait for them to drain.
//
// Without this manager, runImportAnalysis would launch on a bare goroutine
// with context.Background(), making it impossible to cancel mid-analysis on
// server shutdown — the goroutine could keep writing to the database while
// the database is being closed.
type importJobManager struct {
	mu           sync.Mutex
	cancel       context.CancelFunc
	jobCtx       context.Context //nolint:containedctx // mirrors workers.go: ctx is captured-and-shared by per-job closures, not used by struct methods
	shutdown     bool
	wg           sync.WaitGroup
	logger       *slog.Logger
	lastTickUnix int64
	active       atomic.Int64

	// run is the work function invoked per submission. It is a field rather
	// than a hard-coded call so tests can substitute a fake without spinning
	// up the full analyzer.
	run func(ctx context.Context, importID, backupPath string)
}

var importJobManagerExpvarOnce sync.Once

// newImportJobManager constructs a manager whose context derives from the
// supplied parent. If parent is nil, context.Background is used.
func newImportJobManager(parent context.Context, logger *slog.Logger, run func(ctx context.Context, importID, backupPath string)) *importJobManager {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	m := &importJobManager{
		cancel: cancel,
		jobCtx: ctx,
		logger: logger,
		run:    run,
	}
	atomic.StoreInt64(&m.lastTickUnix, time.Now().Unix())
	importJobManagerExpvarOnce.Do(func() {
		expvar.Publish("import_jobs_last_tick_unix", expvar.Func(func() any {
			return atomic.LoadInt64(&m.lastTickUnix)
		}))
		expvar.Publish("import_jobs_active", expvar.Func(func() any {
			return m.active.Load()
		}))
	})
	return m
}

// Submit launches an import analysis on a tracked goroutine. If the manager
// has already been shut down, Submit logs and returns without launching a
// goroutine.
func (m *importJobManager) Submit(importID, backupPath string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.shutdown {
		m.mu.Unlock()
		if m.logger != nil {
			m.logger.Warn("import job manager rejected submission after shutdown",
				slog.String("import_id", importID))
		}
		return
	}
	ctx := m.jobCtx
	m.wg.Add(1)
	m.mu.Unlock()

	go func() {
		defer m.wg.Done()
		m.active.Add(1)
		defer m.active.Add(-1)
		atomic.StoreInt64(&m.lastTickUnix, time.Now().Unix())
		m.run(ctx, importID, backupPath)
	}()
}

// LastTick returns the wall-clock time of the most recent loop iteration.
func (m *importJobManager) LastTick() time.Time {
	return time.Unix(atomic.LoadInt64(&m.lastTickUnix), 0)
}

// ActiveJobs returns the number of currently running import analysis jobs.
func (m *importJobManager) ActiveJobs() int64 {
	return m.active.Load()
}

// Shutdown cancels the manager's context and waits for active analyses to
// return. If the supplied context's deadline elapses before all workers
// finish, Shutdown returns the context's error; goroutines remain running
// but should observe the cancellation and exit on their own.
func (m *importJobManager) Shutdown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	m.shutdown = true
	m.mu.Unlock()
	m.cancel()

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return errors.Join(ctx.Err(), errors.New("import job manager: shutdown timed out waiting for analyses"))
	}
}
