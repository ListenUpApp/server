package abs

import (
	"sync"
	"time"
)

// AnalysisPhase represents the current phase of analysis.
type AnalysisPhase string

const (
	PhaseParsing          AnalysisPhase = "parsing"
	PhaseMatchingUsers    AnalysisPhase = "matching_users"
	PhaseMatchingBooks    AnalysisPhase = "matching_books"
	PhaseMatchingSessions AnalysisPhase = "matching_sessions"
	PhaseMatchingProgress AnalysisPhase = "matching_progress"
	PhaseDone             AnalysisPhase = "done"
)

// AnalysisStatus represents the overall status of an async analysis.
type AnalysisStatus string

const (
	StatusRunning   AnalysisStatus = "running"
	StatusCompleted AnalysisStatus = "completed"
	StatusFailed    AnalysisStatus = "failed"
)

// AnalysisProgress tracks the progress of an async analysis.
type AnalysisProgress struct {
	mu      sync.RWMutex
	Status  AnalysisStatus `json:"status"`
	Phase   AnalysisPhase  `json:"phase"`
	Current int            `json:"current"`
	Total   int            `json:"total"`
	Result  interface{}    `json:"result,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// Update sets the current progress phase and counts.
func (p *AnalysisProgress) Update(phase AnalysisPhase, current, total int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Phase = phase
	p.Current = current
	p.Total = total
}

// Complete marks the analysis as completed with a result.
func (p *AnalysisProgress) Complete(result interface{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Status = StatusCompleted
	p.Phase = PhaseDone
	p.Result = result
}

// Fail marks the analysis as failed with an error message.
func (p *AnalysisProgress) Fail(err string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Status = StatusFailed
	p.Error = err
}

// Snapshot returns a copy of the current progress state.
func (p *AnalysisProgress) Snapshot() AnalysisProgress {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return AnalysisProgress{
		Status:  p.Status,
		Phase:   p.Phase,
		Current: p.Current,
		Total:   p.Total,
		Result:  p.Result,
		Error:   p.Error,
	}
}

// ProgressCallback is called by the analyzer to report progress.
type ProgressCallback func(phase AnalysisPhase, current, total int)

// AnalysisTracker manages in-flight async analyses.
type AnalysisTracker struct {
	mu       sync.RWMutex
	analyses map[string]*AnalysisProgress
}

// NewAnalysisTracker creates a new tracker.
func NewAnalysisTracker() *AnalysisTracker {
	return &AnalysisTracker{
		analyses: make(map[string]*AnalysisProgress),
	}
}

// Start creates a new tracked analysis and returns its progress.
func (t *AnalysisTracker) Start(id string) *AnalysisProgress {
	progress := &AnalysisProgress{
		Status: StatusRunning,
		Phase:  PhaseParsing,
	}
	t.mu.Lock()
	t.analyses[id] = progress
	t.mu.Unlock()
	return progress
}

// Get returns the progress for an analysis, or nil if not found.
func (t *AnalysisTracker) Get(id string) *AnalysisProgress {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.analyses[id]
}

// Cleanup removes completed analyses older than the given duration.
func (t *AnalysisTracker) Cleanup(maxAge time.Duration) {
	// Simple approach: remove all completed/failed analyses
	// In practice, completed analyses are fetched once and then
	// the client moves on to the import hub.
	t.mu.Lock()
	defer t.mu.Unlock()
	for id, p := range t.analyses {
		snap := p.Snapshot()
		if snap.Status == StatusCompleted || snap.Status == StatusFailed {
			delete(t.analyses, id)
		}
	}
	_ = maxAge // reserved for future time-based cleanup
}
