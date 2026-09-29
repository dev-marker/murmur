package jobs

import (
	"slices"
	"sync"
	"time"
)

// A tally collects how long finished jobs waited, until somebody takes them.
type tally struct {
	mu    sync.Mutex
	waits []time.Duration
}

func (t *tally) add(wait time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.waits = append(t.waits, wait)
}

// take hands over the waits, shortest first, and starts a new page.
func (t *tally) take() []time.Duration {
	t.mu.Lock()
	waits := t.waits
	t.waits = nil
	t.mu.Unlock()
	slices.Sort(waits)
	return waits
}
