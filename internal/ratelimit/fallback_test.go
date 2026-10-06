package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"
)

// flaky is a limiter we can switch off, the way Redis is when it is stopped.
type flaky struct {
	*Limiter
	down  bool
	asked int
}

func (f *flaky) Allow(ctx context.Context, key string) (Result, error) {
	f.asked++
	if f.down {
		return Result{}, errors.New("redis is away")
	}
	return f.Limiter.Allow(ctx, key)
}

// fallback returns a Fallback from a flaky limiter to one in memory, both with a
// burst of two, and the clock all three of them share.
func fallback() (*Fallback, *flaky, *time.Time) {
	clock := noon
	now := func() time.Time { return clock }
	first, spare := &flaky{Limiter: New("test", 2, 60)}, New("test", 2, 60)
	first.now, spare.now = now, now
	f := NewFallback(first, spare)
	f.now = now
	return f, first, &clock
}

func TestFallback(t *testing.T) {
	f, first, clock := fallback()
	steps := []struct {
		wait  time.Duration
		down  bool // is the first limiter away?
		left  int  // tokens left in the bucket that answered
		asked int  // how often the first limiter has been asked so far
	}{
		{0, false, 1, 1},           // the shared bucket: two, minus one
		{0, true, 1, 2},            // it fails, and the spare answers from a bucket of its own
		{0, true, 0, 2},            // for a second, nobody asks the first limiter again
		{time.Second, true, 0, 3},  // then one request goes and looks, and falls back again
		{time.Second, false, 1, 4}, // it is back, and its bucket has had two seconds to fill
		{0, false, 0, 5},           // from here on it is asked every time
	}
	for i, s := range steps {
		*clock = clock.Add(s.wait)
		first.down = s.down
		res, err := f.Allow(context.Background(), "ann")
		if err != nil || !res.Allowed || res.Remaining != s.left || first.asked != s.asked {
			t.Fatalf("step %d: got %+v (%v) after asking %d times, want %d left after %d",
				i+1, res, err, first.asked, s.left, s.asked)
		}
	}
}

// stuck is a limiter that never answers, the way Redis is when it hangs.
type stuck struct{ Allower }

func (stuck) Allow(ctx context.Context, _ string) (Result, error) {
	<-ctx.Done()
	return Result{}, ctx.Err()
}

func TestFallbackDoesNotWait(t *testing.T) {
	f, first, _ := fallback()
	f.Allower, f.patience = stuck{first}, time.Millisecond
	began := time.Now()
	res, err := f.Allow(context.Background(), "ann")
	if err != nil || !res.Allowed {
		t.Fatalf("got %+v (%v), want a yes from the spare", res, err)
	}
	if took := time.Since(began); took > time.Second {
		t.Fatalf("the answer took %v", took)
	}
}
