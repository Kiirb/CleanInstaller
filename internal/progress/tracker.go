// Package progress tracks weighted multi-stage progress. Workers call Begin
// and Set; the UI polls Snapshot.
package progress

import "sync"

// Snapshot is what the UI shows.
type Snapshot struct {
	Percent float64 // 0..100
	Status  string
	Done    bool
}

// Tracker is safe for concurrent use.
type Tracker struct {
	mu      sync.Mutex
	weights []float64 // normalised to sum 100
	stage   int       // index of current stage, -1 before the first Begin
	base    float64   // percent completed by earlier stages
	frac    float64   // 0..1 within the current stage
	status  string
	done    bool
}

// NewTracker returns a tracker with no stages planned.
func NewTracker() *Tracker { return &Tracker{stage: -1} }

// Plan declares all stage weights up front; they are normalised to 100 and
// progress is reset.
func (t *Tracker) Plan(weights ...float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var sum float64
	for _, w := range weights {
		sum += w
	}
	t.weights = make([]float64, len(weights))
	for i, w := range weights {
		if sum > 0 {
			t.weights[i] = w / sum * 100
		}
	}
	t.stage, t.base, t.frac, t.done = -1, 0, 0, false
}

func (t *Tracker) weight(i int) float64 {
	if i >= 0 && i < len(t.weights) {
		return t.weights[i]
	}
	return 0
}

// Begin completes the current stage and starts the next declared one.
func (t *Tracker) Begin(status string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stage >= 0 {
		t.base += t.weight(t.stage)
	}
	t.stage++
	t.frac = 0
	t.status = status
}

// Set reports progress within the current stage (0..1). It never moves back.
func (t *Tracker) Set(fraction float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if fraction > 1 {
		fraction = 1
	}
	if fraction > t.frac {
		t.frac = fraction
	}
}

// Status replaces the status text without changing progress.
func (t *Tracker) Status(s string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status = s
}

// Finish marks all work done.
func (t *Tracker) Finish() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.done = true
}

// Snapshot returns the current state.
func (t *Tracker) Snapshot() Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.done {
		return Snapshot{Percent: 100, Status: t.status, Done: true}
	}
	p := t.base + t.weight(t.stage)*t.frac
	if p > 99.9 {
		p = 99.9
	}
	if t.stage >= 0 && t.stage == len(t.weights)-1 && t.frac >= 1 {
		p = 100
	}
	return Snapshot{Percent: p, Status: t.status}
}
