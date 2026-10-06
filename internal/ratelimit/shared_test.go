package ratelimit

import (
	"context"
	"testing"
	"time"
)

// In Redis nobody sweeps. Every bucket carries its own expiry: as long as it takes to fill.
func TestSharedBucketExpires(t *testing.T) {
	s, _ := shared(t, 3, 60)
	s.Allow(context.Background(), "ann")
	ttl := s.rdb.PTTL(context.Background(), "ratelimit:test:ann").Val()
	if ttl <= 0 || ttl > 3*time.Second {
		t.Fatalf("the bucket expires in %v, want within three seconds", ttl)
	}
}

// Two servers have two clocks. One that runs behind must not hand out tokens,
// and it must not turn the bucket's own clock back.
func TestSharedClockBehind(t *testing.T) {
	s, clock := shared(t, 1, 60)
	ask := func() bool {
		res, err := s.Allow(context.Background(), "ann")
		if err != nil {
			t.Fatal(err)
		}
		return res.Allowed
	}
	if !ask() {
		t.Fatal("the first request was refused")
	}
	*clock = clock.Add(-10 * time.Second) // a server whose clock is ten seconds behind
	if ask() {
		t.Fatal("a clock that runs behind got a token")
	}
	*clock = clock.Add(10*time.Second + 500*time.Millisecond) // the first server, half a second on
	if ask() {
		t.Fatal("half a second bought a whole token")
	}
}
