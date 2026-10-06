package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// take is Allow, as one script inside Redis.
// KEYS[1] is the bucket. ARGV is the burst, the tokens a minute, and the time in milliseconds.
var take = redis.NewScript(`
local burst, rate, now = tonumber(ARGV[1]), tonumber(ARGV[2]) / 60, tonumber(ARGV[3])
local b = redis.call('HMGET', KEYS[1], 'tokens', 'seen')
local tokens, seen = tonumber(b[1]) or burst, tonumber(b[2]) or now
tokens = math.min(burst, tokens + math.max(0, now - seen) / 1000 * rate)
local allowed, wait = 0, 0
if tokens >= 1 then
	tokens = tokens - 1
	allowed = 1
else
	wait = math.ceil((1 - tokens) / rate * 1000)
end
redis.call('HSET', KEYS[1], 'tokens', tokens, 'seen', math.max(now, seen))
redis.call('PEXPIRE', KEYS[1], math.ceil(burst / rate * 1000))
return {allowed, math.floor(tokens), wait}`)

// A Shared limiter keeps its buckets in Redis, where every API server finds the same ones.
type Shared struct {
	name      string
	burst     int
	perMinute int
	now       func() time.Time
	rdb       *redis.Client
}

// NewShared returns a Shared limiter for the policy called name, with its buckets in rdb.
func NewShared(rdb *redis.Client, name string, burst, perMinute int) *Shared {
	return &Shared{name: name, burst: burst, perMinute: perMinute, now: time.Now, rdb: rdb}
}

// Policy is what this limit is called, and how many tokens a bucket gains in a minute.
func (s *Shared) Policy() (name string, perMinute int) {
	return s.name, s.perMinute
}

// Allow takes one token from key's bucket in Redis, if there is one.
func (s *Shared) Allow(ctx context.Context, key string) (Result, error) {
	keys := []string{"ratelimit:" + s.name + ":" + key}
	now := s.now().UnixMilli()
	v, err := take.Run(ctx, s.rdb, keys, s.burst, s.perMinute, now).Int64Slice()
	if err != nil {
		return Result{}, err
	}
	wait := time.Duration(v[2]) * time.Millisecond
	return Result{Allowed: v[0] == 1, Remaining: int(v[1]), RetryAfter: wait}, nil
}
