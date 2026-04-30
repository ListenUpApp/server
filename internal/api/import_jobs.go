package api

import (
	"context"
	"errors"
	"log/slog"
	"sync"
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
	mu       sync.Mutex
	cancel   context.CancelFunc
	jobCtx   context.Context //nolint:containedctx // mirrors workers.go: ctx is captured-and-shared by per-job closures, not used by struct methods
	shutdown bool
	wg       sync.WaitGroup
	logger   *slog.Logger

	// run is the work function invoked per submission. It is a field rather
	// than a hard-coded call so tests can substitute a fake without spinning
	// up the full analyzer.
	run func(ctx context.Context, importID, backupPath string)
}

// newImportJobManager constructs a manager whose context derives from the
// supplied parent. If parent is nil, context.Background is used.
func newImportJobManager(parent context.Context, logger *slog.Logger, run func(ctx context.Context, importID, backupPath string)) *importJobManager {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	return &importJobManager{
		cancel: cancel,
		jobCtx: ctx,
		logger: logger,
		run:    run,
	}
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
		m.run(ctx, importID, backupPath)
	}()
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
