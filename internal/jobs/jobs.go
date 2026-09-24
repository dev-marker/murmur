// Package jobs is a job queue in Postgres: work that is saved now, and done later by a worker.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	_, err := db.Exec(ctx, `insert into jobs (kind, payload) values ($1, $2)`, kind, payload)
	return err
}

// A Handler does one kind of job. An error means the job is tried again.
type Handler func(ctx context.Context, payload []byte) error

// A Worker claims jobs, one at a time, and runs them.
type Worker struct {
	db       *pgxpool.Pool
	handlers map[string]Handler
}

func NewWorker(db *pgxpool.Pool) *Worker {
	return &Worker{db: db, handlers: map[string]Handler{}}
}

// Handle says which func runs jobs of this kind.
func (w *Worker) Handle(kind string, h Handler) {
	w.handlers[kind] = h
}

type job struct {
	id      int64
	kind    string
	payload []byte
}

// claim takes the oldest waiting job and marks it running, in one statement.
func (w *Worker) claim(ctx context.Context) (job, error) {
	var j job
	err := w.db.QueryRow(ctx, `
		update jobs set status = 'running'
		where id = (
			select id
			from jobs
			where status = 'waiting'
			order by id
			limit 1
			for update skip locked
		)
		returning id, kind, payload`).Scan(&j.id, &j.kind, &j.payload)
	return j, err
}

// RunOne claims one job and runs it. It returns false when no job is waiting.
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
		// The job isn't lost. It goes back to waiting, and a later round tries it again.
		_, back := w.db.Exec(ctx, `update jobs set status = 'waiting' where id = $1`, j.id)
		return true, errors.Join(fmt.Errorf("job %d (%s): %w", j.id, j.kind, err), back)
	}
	_, err = w.db.Exec(ctx, `delete from jobs where id = $1`, j.id)
	return true, err
}

// Run works through the queue until ctx is cancelled.
func (w *Worker) Run(ctx context.Context, poll time.Duration) {
	for ctx.Err() == nil {
		start, done := time.Now(), 0
		for ctx.Err() == nil {
			// A job that has started is finished, even after a cancel.
			ok, err := w.RunOne(context.WithoutCancel(ctx))
			if err != nil {
				slog.Error("job failed", "error", err)
			}
			if !ok || err != nil {
				break
			}
			done++
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
