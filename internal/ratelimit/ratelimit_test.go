package ratelimit

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fiston-user/murmur/internal/platform/testdb"
)

// noon is where every test's clock starts. It stands still until the test moves it.
var noon = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

// limiter returns a Limiter in memory, and its clock.
func limiter(burst, perMinute int) (*Limiter, *time.Time) {
	clock := noon
	l := New("test", burst, perMinute)
	l.now = func() time.Time { return clock }
	return l, &clock
}

// shared returns a Shared limiter on the Compose Redis (make up), and its clock.
func shared(t *testing.T, burst, perMinute int) (*Shared, *time.Time) {
	clock := noon
	s := NewShared(testdb.Redis(t), "test", burst, perMinute)
	s.now = func() time.Time { return clock }
	return s, &clock
}

// both runs one test twice: on the limiter in memory, and on the one in Redis.
// Whatever a limiter promises, both of them have to keep.
func both(t *testing.T, burst, perMinute int, test func(t *testing.T, l Allower, clock *time.Time)) {
	t.Run("memory", func(t *testing.T) {
		l, clock := limiter(burst, perMinute)
		test(t, l, clock)
	})
	t.Run("redis", func(t *testing.T) {
		l, clock := shared(t, burst, perMinute)
		test(t, l, clock)
	})
}

func TestAllow(t *testing.T) {
	// three at once, and then one a second
	both(t, 3, 60, func(t *testing.T, l Allower, clock *time.Time) {
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
			got, err := l.Allow(context.Background(), s.key)
			if err != nil || got != s.want {
				t.Fatalf("step %d: %s got %+v (%v), want %+v", i+1, s.key, got, err, s.want)
			}
		}
	})
}

// A hundred goroutines ask for the same key at the same moment, ten times each.
// Exactly the burst gets through.
func TestCrowd(t *testing.T) {
	both(t, 20, 60, func(t *testing.T, l Allower, _ *time.Time) {
		var allowed atomic.Int64
		var crowd sync.WaitGroup
		start := make(chan struct{})
		for range 100 {
			crowd.Go(func() {
				<-start
				for range 10 {
					if res, _ := l.Allow(context.Background(), "ann"); res.Allowed {
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
	})
}

// A bucket in memory that nobody has used for long enough to be full again is forgotten.
func TestIdleBucketsAreForgotten(t *testing.T) {
	l, clock := limiter(3, 60) // an empty bucket is full again after three seconds
	for i := range 1000 {
		l.Allow(context.Background(), strconv.Itoa(i))
	}
	*clock = clock.Add(3 * time.Second)
	l.Allow(context.Background(), "late")
	if n := len(l.buckets); n != 1 {
		t.Fatalf("the limiter holds %d buckets, want only the one that just asked", n)
	}
}
