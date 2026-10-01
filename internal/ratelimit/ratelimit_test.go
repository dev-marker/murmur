package ratelimit

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// limiter returns a Limiter on a clock that stands still until the test moves it.
func limiter(burst, perMinute int) (*Limiter, *time.Time) {
	clock := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	l := New("test", burst, perMinute)
	l.now = func() time.Time { return clock }
	return l, &clock
}

func TestAllow(t *testing.T) {
	l, clock := limiter(3, 60) // three at once, and then one a second
	steps := []struct {
		wait time.Duration // the clock moves this far, and then key asks
		key  string
		want Result
	}{
		{0, "ann", Result{true, 2, 0}},
		{0, "ann", Result{true, 1, 0}},
		{0, "ann", Result{true, 0, 0}},
		{0, "ann", Result{false, 0, time.Second}},
		{0, "bob", Result{true, 2, 0}},
		{250 * time.Millisecond, "ann", Result{false, 0, 750 * time.Millisecond}},
		{750 * time.Millisecond, "ann", Result{true, 0, 0}},
		{2500 * time.Millisecond, "ann", Result{true, 1, 0}},
		{time.Hour, "ann", Result{true, 2, 0}},
	}
	for i, s := range steps {
		*clock = clock.Add(s.wait)
		if got := l.Allow(s.key); got != s.want {
			t.Fatalf("step %d: %s got %+v, want %+v", i+1, s.key, got, s.want)
		}
	}
}

// A hundred goroutines ask for the same key at the same moment, ten times each.
// Exactly the burst gets through.
func TestCrowd(t *testing.T) {
	l, _ := limiter(20, 60)
	var allowed atomic.Int64
	var crowd sync.WaitGroup
	start := make(chan struct{})
	for range 100 {
		crowd.Go(func() {
			<-start
			for range 10 {
				if l.Allow("ann").Allowed {
					allowed.Add(1)
				}
			}
		})
	}
	close(start)
	crowd.Wait()
	if got := allowed.Load(); got != 20 {
		t.Fatalf("allowed = %d, want the burst of 20", got)
	}
}

// A bucket nobody has used for long enough to be full again is forgotten.
func TestIdleBucketsAreForgotten(t *testing.T) {
	l, clock := limiter(3, 60) // an empty bucket is full again after three seconds
	for i := range 1000 {
		l.Allow(strconv.Itoa(i))
	}
	*clock = clock.Add(3 * time.Second)
	l.Allow("late")
	if n := len(l.buckets); n != 1 {
		t.Fatalf("the limiter holds %d buckets, want only the one that just asked", n)
	}
}
