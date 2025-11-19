package scanner

import (
	"sync"
	"sync/atomic"
	"time"
)

// ProgressTracker tracks and reports scan progress with throttled updates.
type ProgressTracker struct {
	callback func(*Progress)

	// Atomic counters for hot path (no lock needed)
	current atomic.Int64
	total   atomic.Int64

	// Mutex only for string fields and error slice
	mu          sync.Mutex
	phase       ScanPhase
	currentItem string
	errors      []ScanError
	added       atomic.Int64
	updated     atomic.Int64
	removed     atomic.Int64

	// Throttling mechanism
	lastNotify   time.Time
	throttle     time.Duration
	pendingDirty atomic.Bool // Indicates pending update

	// Shutdown mechanism
	done chan struct{}
	wg   sync.WaitGroup
}

// NewProgressTracker creates a new progress tracker with throttled updates.
// Updates are throttled to at most once per 100ms to avoid goroutine spam.
func NewProgressTracker(callback func(*Progress)) *ProgressTracker {
	p := &ProgressTracker{
		callback:   callback,
		phase:      PhaseWalking,
		throttle:   100 * time.Millisecond,
		lastNotify: time.Now(),
		done:       make(chan struct{}),
	}

	// Start background update goroutine
	p.wg.Add(1)
	go p.updateLoop()

	return p
}

// updateLoop runs in the background and sends throttled updates.
func (p *ProgressTracker) updateLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.throttle)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Check if there's a pending update
			if p.pendingDirty.CompareAndSwap(true, false) {
				p.sendUpdate()
			}
		case <-p.done:
			// Final update on shutdown
			if p.pendingDirty.Load() {
				p.sendUpdate()
			}
			return
		}
	}
}

// sendUpdate sends the current progress to the callback (under lock).
func (p *ProgressTracker) sendUpdate() {
	if p.callback == nil {
		return
	}

	p.mu.Lock()
	progress := Progress{
		Phase:       p.phase,
		CurrentItem: p.currentItem,
		Errors:      append([]ScanError(nil), p.errors...), // Copy slice
		Current:     int(p.current.Load()),
		Total:       int(p.total.Load()),
		Added:       int(p.added.Load()),
		Updated:     int(p.updated.Load()),
		Removed:     int(p.removed.Load()),
	}
	p.mu.Unlock()

	// Call callback in same goroutine (updateLoop) - no goroutine spawn
	p.callback(&progress)
	p.lastNotify = time.Now()
}

// SetPhase updates the current phase.
func (p *ProgressTracker) SetPhase(phase ScanPhase) {
	p.mu.Lock()
	p.phase = phase
	p.mu.Unlock()

	p.current.Store(0)
	p.total.Store(0)
	p.markDirty()
}

// SetTotal sets the total items for current phase.
func (p *ProgressTracker) SetTotal(total int) {
	p.total.Store(int64(total))
	p.markDirty()
}

// Increment increments the current progress.
// This is the hot path - uses atomic increment, no lock.
func (p *ProgressTracker) Increment(currentItem string) {
	p.current.Add(1)

	// Only lock for string update
	if currentItem != "" {
		p.mu.Lock()
		p.currentItem = currentItem
		p.mu.Unlock()
	}

	p.markDirty()
}

// AddError records an error.
func (p *ProgressTracker) AddError(err ScanError) {
	p.mu.Lock()
	p.errors = append(p.errors, err)
	p.mu.Unlock()

	p.markDirty()
}

// IncrementAdded increments the added counter (for diff phase).
func (p *ProgressTracker) IncrementAdded() {
	p.added.Add(1)
	p.markDirty()
}

// IncrementUpdated increments the updated counter (for diff phase).
func (p *ProgressTracker) IncrementUpdated() {
	p.updated.Add(1)
	p.markDirty()
}

// IncrementRemoved increments the removed counter (for diff phase).
func (p *ProgressTracker) IncrementRemoved() {
	p.removed.Add(1)
	p.markDirty()
}

// Get returns current progress snapshot.
func (p *ProgressTracker) Get() Progress {
	p.mu.Lock()
	defer p.mu.Unlock()

	return Progress{
		Phase:       p.phase,
		CurrentItem: p.currentItem,
		Errors:      append([]ScanError(nil), p.errors...), // Copy
		Current:     int(p.current.Load()),
		Total:       int(p.total.Load()),
		Added:       int(p.added.Load()),
		Updated:     int(p.updated.Load()),
		Removed:     int(p.removed.Load()),
	}
}

// markDirty marks that an update is pending.
func (p *ProgressTracker) markDirty() {
	p.pendingDirty.Store(true)
}

// Close shuts down the progress tracker and sends final update.
func (p *ProgressTracker) Close() {
	close(p.done)
	p.wg.Wait()
}
