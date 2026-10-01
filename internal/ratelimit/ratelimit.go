// Package ratelimit is a token bucket rate limiter that lives in memory.
package ratelimit

import (
	"sync"
	"time"
)

// A Limiter keeps one bucket of tokens per key.
type Limiter struct {
	name      string
	burst     int
	perMinute int
	now       func() time.Time

	mu      sync.Mutex
	buckets map[string]bucket
	swept   time.Time
}

// A bucket is all we keep about one key.
type bucket struct {
	tokens float64
	seen   time.Time
}

// New returns a Limiter for the policy called name. Its buckets hold burst tokens,
// and they gain perMinute tokens a minute.
func New(name string, burst, perMinute int) *Limiter {
	return &Limiter{
		name:      name,
		burst:     burst,
		perMinute: perMinute,
		now:       time.Now,
		buckets:   make(map[string]bucket),
	}
}

// Result is the answer to one request.
type Result struct {
	Allowed    bool
	Remaining  int
	RetryAfter time.Duration
}

// Allow takes one token from key's bucket, if there is one.
func (l *Limiter) Allow(key string) Result {
	burst, rate := float64(l.burst), float64(l.perMinute)/60
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Sub(l.swept) >= l.full() {
		l.sweep(now)
	}
	b, ok := l.buckets[key]
	if !ok {
		b = bucket{tokens: burst, seen: now}
	}
	b.tokens = min(burst, b.tokens+now.Sub(b.seen).Seconds()*rate)
	b.seen = now

	var res Result
	if b.tokens >= 1 {
		b.tokens--
		res.Allowed = true
	} else {
		res.RetryAfter = time.Duration((1 - b.tokens) / rate * float64(time.Second))
	}
	res.Remaining = int(b.tokens)
	l.buckets[key] = b
	return res
}

// full is how long an empty bucket takes to fill up.
func (l *Limiter) full() time.Duration {
	return time.Duration(l.burst) * time.Minute / time.Duration(l.perMinute)
}

// sweep forgets every bucket that has been idle for that long. It is full again.
func (l *Limiter) sweep(now time.Time) {
	for key, b := range l.buckets {
		if now.Sub(b.seen) >= l.full() {
			delete(l.buckets, key)
		}
	}
	l.swept = now
}
