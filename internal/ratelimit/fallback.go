package ratelimit

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

// A Fallback asks the shared limiter first. When that one fails, or takes too long,
// the spare answers, and the shared one is left alone for a moment.
type Fallback struct {
	Allower                // the shared buckets, in Redis
	spare    Allower       // this server's own buckets, in memory
	patience time.Duration // how long the shared limiter gets to answer
	pause    time.Duration // how long it is left alone after a failure
	now      func() time.Time
	until    atomic.Int64 // the end of that pause in unix nanoseconds, or zero
}

// NewFallback returns shared, with spare to fall back on.
func NewFallback(shared, spare Allower) *Fallback {
	return &Fallback{
		Allower:  shared,
		spare:    spare,
		patience: 250 * time.Millisecond,
		pause:    time.Second,
		now:      time.Now,
	}
}

// Allow takes a token from the shared bucket, or from the spare one while Redis is away.
func (f *Fallback) Allow(ctx context.Context, key string) (Result, error) {
	now, until := f.now(), f.until.Load()
	next := now.Add(f.pause).UnixNano()
	if until != 0 && (now.UnixNano() < until || !f.until.CompareAndSwap(until, next)) {
		return f.spare.Allow(ctx, key)
	}
	brief, cancel := context.WithTimeout(ctx, f.patience)
	defer cancel()
	res, err := f.Allower.Allow(brief, key)
	name, _ := f.Policy()
	if err == nil {
		if until != 0 {
			f.until.Store(0)
			slog.Info("rate limit is shared again", "limit", name)
		}
		return res, nil
	}
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	if until == 0 && f.until.CompareAndSwap(0, next) {
		slog.Warn("rate limit falls back to this server's memory", "limit", name, "error", err)
	}
	return f.spare.Allow(ctx, key)
}
