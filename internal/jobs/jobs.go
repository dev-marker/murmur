// Package jobs is a job queue in Postgres: work that is saved now, and done later by a worker.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB is what Enqueue writes through: a pool, or a transaction.
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Enqueue saves a job. In a transaction, it's saved only if that commits.
func Enqueue(ctx context.Context, db DB, kind string, payload any) error {
	_, err := db.Exec(ctx, `insert into jobs (kind, payload, run_at) values ($1, $2, $3)`,
		kind, payload, time.Now())
	return err
}

// MaxAttempts is how often a job is tried before it's dead.
const MaxAttempts = 5

// The wait before the next try starts at baseWait, doubles every time, and stops at maxWait.
const (
	baseWait = time.Second
	maxWait  = time.Minute
)

// backoff is how long a job waits after a failed attempt. r is a random number,
// from 0 up to 1, and it picks a wait between half of the full one and all of it.
func backoff(attempt int, r float64) time.Duration {
	wait := baseWait
	for i := 1; i < attempt && wait < maxWait; i++ {
		wait *= 2
	}
	wait = min(wait, maxWait)
	return wait/2 + time.Duration(r*float64(wait/2))
}

// A Handler does one kind of job. An error means the job is tried again.
type Handler func(ctx context.Context, payload []byte) error

// A Worker claims jobs, one at a time, and runs them.
type Worker struct {
	db       *pgxpool.Pool
	handlers map[string]Handler
	now      func() time.Time // the worker's clock. A test can set its own
}

func NewWorker(db *pgxpool.Pool) *Worker {
	return &Worker{db: db, handlers: map[string]Handler{}, now: time.Now}
}

// Handle says which func runs jobs of this kind.
func (w *Worker) Handle(kind string, h Handler) {
	w.handlers[kind] = h
}

type job struct {
	id      int64
	kind    string
	payload []byte
	attempt int // which try this is, counted by the claim
}

// claim takes the waiting job that is due first and marks it running, in one statement.
func (w *Worker) claim(ctx context.Context) (job, error) {
	var j job
	err := w.db.QueryRow(ctx, `
		update jobs set status = 'running', attempts = attempts + 1, claimed_at = $1
		where id = (
			select id
			from jobs
			where status = 'waiting' and run_at <= $1
			order by run_at, id
			limit 1
			for update skip locked
		)
		returning id, kind, payload, attempts`, w.now()).Scan(&j.id, &j.kind, &j.payload, &j.attempt)
	return j, err
}

// fail writes down a failed attempt. The job waits for its next try, or it's dead.
func (w *Worker) fail(ctx context.Context, j job, cause error) error {
	if j.attempt >= MaxAttempts {
		_, err := w.db.Exec(ctx, `update jobs set status = 'dead', last_error = $2 where id = $1`,
			j.id, cause.Error())
		return err
	}
	at := w.now().Add(backoff(j.attempt, rand.Float64()))
	_, err := w.db.Exec(ctx, `
		update jobs set status = 'waiting', run_at = $2, last_error = $3
		where id = $1`, j.id, at, cause.Error())
	return err
}

// Sweep takes back every job that has been running for longer than timeout,
// because its worker is gone. It returns how many jobs that were.
func (w *Worker) Sweep(ctx context.Context, timeout time.Duration) (int64, error) {
	tag, err := w.db.Exec(ctx, `
		update jobs
		set status = case when attempts >= $2 then 'dead' else 'waiting' end,
			last_error = 'its worker went away'
		where status = 'running' and claimed_at < $1`, w.now().Add(-timeout), MaxAttempts)
	return tag.RowsAffected(), err
}

// RunOne claims one job and runs it. It returns false when no job is due.
func (w *Worker) RunOne(ctx context.Context) (bool, error) {
	j, err := w.claim(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim a job: %w", err)
	}
	h, ok := w.handlers[j.kind]
	if !ok {
		err = errors.New("no handler")
	} else {
		err = h(ctx, j.payload)
	}
	if err != nil {
		back := w.fail(ctx, j, err)
		err = fmt.Errorf("job %d (%s), attempt %d of %d: %w", j.id, j.kind, j.attempt, MaxAttempts, err)
		return true, errors.Join(err, back)
	}
	_, err = w.db.Exec(ctx, `delete from jobs where id = $1`, j.id)
	return true, err
}

// Run works through the queue until ctx is cancelled. A job that has been
// running for longer than timeout goes back to the queue.
func (w *Worker) Run(ctx context.Context, poll, timeout time.Duration) {
	var swept time.Time
	for ctx.Err() == nil {
		start, done := time.Now(), 0
		for ctx.Err() == nil {
			if time.Since(swept) > timeout/2 {
				swept = time.Now()
				n, err := w.Sweep(ctx, timeout)
				if err != nil {
					slog.Error("sweep failed", "error", err)
				} else if n > 0 {
					slog.Info("jobs swept", "count", n)
				}
			}
			// A job that has started is finished, even after a cancel.
			ok, err := w.RunOne(context.WithoutCancel(ctx))
			if err != nil {
				slog.Error("job failed", "error", err)
			}
			if !ok {
				break
			}
			if err == nil {
				done++
			}
		}
		if done > 0 {
			took := time.Since(start).Round(time.Millisecond)
			slog.Info("jobs done", "count", done, "took", took.String())
		}
		select {
		case <-ctx.Done():
		case <-time.After(poll):
		}
	}
}
