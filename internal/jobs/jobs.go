// Package jobs is a job queue in Postgres: work that is saved now, and done later by a worker.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
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
	_, err := db.Exec(ctx, `insert into jobs (kind, payload, created_at, run_at) values ($1, $2, $3, $3)`,
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
	done     tally
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
	queued  time.Time
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
		returning id, kind, payload, attempts, created_at`, w.now()).
		Scan(&j.id, &j.kind, &j.payload, &j.attempt, &j.queued)
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
	if err == nil {
		w.done.add(w.now().Sub(j.queued))
	}
	return true, err
}

// Run works through the queue with n loops side by side, until ctx is cancelled.
// A job that has been running for longer than timeout goes back to the queue.
// After the cancel, no loop takes a new job, and Run returns when the jobs in hand are done.
// A job that needs longer than grace for that is cut off. A sweep will find it.
func (w *Worker) Run(ctx context.Context, n int, poll, timeout, grace time.Duration) {
	// Jobs get a context of their own, so a cancel doesn't stop them halfway.
	work, cut := context.WithCancel(context.WithoutCancel(ctx))
	defer cut()
	go func() {
		<-ctx.Done()
		select {
		case <-time.After(grace):
			slog.Warn("out of time, jobs in hand are cut off")
			cut()
		case <-work.Done():
		}
	}()
	var loops sync.WaitGroup
	for range n {
		loops.Go(func() { w.loop(ctx, work, poll) })
	}
	idle := make(chan struct{})
	go func() {
		loops.Wait()
		close(idle)
	}()
	w.tend(work, idle, timeout)
}

// tend does what the loops share: it sweeps, and it reports.
func (w *Worker) tend(ctx context.Context, idle <-chan struct{}, timeout time.Duration) {
	sweep, report := time.NewTicker(timeout/2), time.NewTicker(every)
	defer sweep.Stop()
	defer report.Stop()
	w.sweep(ctx, timeout)
	last := time.Now()
	for {
		select {
		case <-sweep.C:
			w.sweep(ctx, timeout)
		case <-report.C:
			w.report(time.Since(last))
			last = time.Now()
		case <-idle:
			w.report(time.Since(last))
			return
		}
	}
}

// loop claims a job and runs it, again and again. With nothing due, it sleeps for poll.
func (w *Worker) loop(ctx, work context.Context, poll time.Duration) {
	for ctx.Err() == nil {
		// A job that has started is finished, even after a cancel.
		ok, err := w.RunOne(work)
		if err != nil {
			slog.Error("job failed", "error", err)
		}
		if ok {
			continue
		}
		select {
		case <-ctx.Done():
		case <-time.After(poll):
		}
	}
}

// sweep is one Sweep, and a line in the log when it found something.
func (w *Worker) sweep(ctx context.Context, timeout time.Duration) {
	n, err := w.Sweep(ctx, timeout)
	if err != nil {
		slog.Error("sweep failed", "error", err)
	} else if n > 0 {
		slog.Info("jobs swept", "count", n)
	}
}

// every is how often a worker reports what it got done.
const every = 5 * time.Second

// report logs what got done since the last report.
func (w *Worker) report(over time.Duration) {
	waits := w.done.take()
	n := len(waits)
	if n == 0 {
		return
	}
	slog.Info("jobs done", "count", n, "over", over.Round(100*time.Millisecond).String(),
		"per_second", int(float64(n)/over.Seconds()),
		"median_wait", waits[n/2].Round(time.Millisecond).String(),
		"longest_wait", waits[n-1].Round(time.Millisecond).String())
}
