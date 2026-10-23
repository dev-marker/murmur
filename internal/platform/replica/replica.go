// Package replica sends reads to a copy of the database, unless they must see
// a write that the copy may not have yet.
package replica

import (
	"context"
	"log/slog"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Fresh is how long a user's reads go to the primary after they wrote.
const Fresh = 10 * time.Second

// Reads decides where a read goes: to the replica, or to the primary.
type Reads struct {
	primary *pgxpool.Pool
	replica *pgxpool.Pool // nil: there is none, and every read goes to the primary
	rdb     *redis.Client
	usable  atomic.Bool // the replica answers, and it is not too far behind
}

func New(primary, replica *pgxpool.Pool, rdb *redis.Client) *Reads {
	return &Reads{primary: primary, replica: replica, rdb: rdb}
}

// key is where Redis remembers, for every server, that a user just wrote.
func key(user int64) string {
	return "wrote:" + strconv.FormatInt(user, 10)
}

// Wrote marks a user who is about to write. It comes before the write,
// so a read that follows the answer finds the mark.
func (r *Reads) Wrote(ctx context.Context, user int64) {
	// If Redis is away, For can't ask either, and it sends the reads to the primary.
	r.rdb.Set(ctx, key(user), 1, Fresh)
}

// For returns the pool for a read that is made for a user, or for nobody (0).
func (r *Reads) For(ctx context.Context, user int64) *pgxpool.Pool {
	if r.replica == nil || !r.usable.Load() {
		return r.primary
	}
	if user != 0 {
		// Did they write a moment ago? If Redis can't say, we take it as a yes.
		if n, err := r.rdb.Exists(ctx, key(user)).Result(); err != nil || n > 0 {
			return r.primary
		}
	}
	return r.replica
}

// Behind is how far the replica may fall behind before nobody reads from it.
// It is well under Fresh, so a write is on the replica before its mark is gone.
const Behind = 3 * time.Second

// Watch asks the replica once a second if it can take reads, until ctx is done.
func (r *Reads) Watch(ctx context.Context) {
	for r.replica != nil {
		r.check(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

// check asks the replica: are you a replica, and have you caught up, or is
// what you haven't replayed yet younger than Behind?
func (r *Reads) check(ctx context.Context) {
	var ok bool
	err := r.replica.QueryRow(ctx, `
		select pg_is_in_recovery() and coalesce(
			pg_last_wal_receive_lsn() = pg_last_wal_replay_lsn()
			or now() - pg_last_xact_replay_timestamp() < $1 * interval '1 second', false)`,
		Behind.Seconds()).Scan(&ok)
	ok = err == nil && ok
	if r.usable.Swap(ok) == ok {
		return
	}
	if ok {
		slog.Info("reads go to the replica")
	} else {
		slog.Warn("reads go to the primary", "error", err)
	}
}
