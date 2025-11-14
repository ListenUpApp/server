package scanner

import "sync"

// ProgressTracker tracks and reports scan progress.
type ProgressTracker struct {
	callback func(*Progress)
	progress Progress
	mu       sync.RWMutex
}

// NewProgressTracker creates a new progress tracker.
func NewProgressTracker(callback func(*Progress)) *ProgressTracker {
	return &ProgressTracker{
		callback: callback,
		progress: Progress{
			Phase: PhaseWalking,
		},
	}
}

// SetPhase updates the current phase.
func (p *ProgressTracker) SetPhase(phase ScanPhase) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.progress.Phase = phase
	p.progress.Current = 0
	p.progress.Total = 0
	p.notify()
}

// SetTotal sets the total items for current phase.
func (p *ProgressTracker) SetTotal(total int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.progress.Total = total
	p.notify()
}

// Increment increments the current progress.
func (p *ProgressTracker) Increment(currentItem string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.progress.Current++
	p.progress.CurrentItem = currentItem
	p.notify()
}

// AddError records an error.
func (p *ProgressTracker) AddError(err ScanError) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.progress.Errors = append(p.progress.Errors, err)
	p.notify()
}

// Get returns current progress.
func (p *ProgressTracker) Get() Progress {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.progress
}

func (p *ProgressTracker) notify() {
	if p.callback != nil {
		// Copy to avoid race.
		progress := p.progress
		go p.callback(&progress)
	}
}
