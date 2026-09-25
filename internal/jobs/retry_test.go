package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fiston-user/murmur/internal/platform/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

// queue is a worker on an empty test database, one job that fails for as long as
// *fails is true, and a clock that stands still until the test moves it.
func queue(t *testing.T) (w *Worker, db *pgxpool.Pool, clock *time.Time, fails *bool) {
	db = testdb.New(t)
	if err := Enqueue(context.Background(), db, "flaky", 1); err != nil {
		t.Fatal(err)
	}
	clock, fails = new(time.Now()), new(true)
	w = NewWorker(db)
	w.now = func() time.Time { return *clock }
	w.Handle("flaky", func(context.Context, []byte) error {
		if *fails {
			return errors.New("not this time")
		}
		return nil
	})
	return w, db, clock, fails
}

// A job that fails is tried again later, and not before.
func TestFailedJobWaits(t *testing.T) {
	w, _, clock, fails := queue(t)
	ctx := context.Background()

	if ok, err := w.RunOne(ctx); !ok || err == nil {
		t.Fatalf("first try: ran = %v, err = %v, want a job and an error", ok, err)
	}
	*fails = false
	if ok, _ := w.RunOne(ctx); ok {
		t.Fatal("the job was tried again at once, want a wait")
	}
	*clock = clock.Add(baseWait)
	if ok, err := w.RunOne(ctx); !ok || err != nil {
		t.Fatalf("after the wait: ran = %v, err = %v, want a job and no error", ok, err)
	}
}

// A job that fails every time is dead after the last attempt, with its error.
func TestDeadAfterLastAttempt(t *testing.T) {
	w, db, clock, _ := queue(t)
	ctx := context.Background()

	for range MaxAttempts {
		if ok, err := w.RunOne(ctx); !ok || err == nil {
			t.Fatalf("ran = %v, err = %v, want a job and an error", ok, err)
		}
		*clock = clock.Add(maxWait)
	}
	if ok, _ := w.RunOne(ctx); ok {
		t.Fatal("a dead job was tried again")
	}
	var status, lastError string
	err := db.QueryRow(ctx, `select status, last_error from jobs`).Scan(&status, &lastError)
	if err != nil || status != "dead" || lastError != "not this time" {
		t.Fatalf("status = %q, last error = %q, err = %v, want a dead job", status, lastError, err)
	}
}

// A worker that dies with a job in its hands leaves it running. The sweep finds it.
func TestStuckJobIsSwept(t *testing.T) {
	w, _, clock, fails := queue(t)
	ctx := context.Background()
	*fails = false

	// The worker claims the job, and that's the last we hear of it.
	if _, err := w.claim(ctx); err != nil {
		t.Fatal(err)
	}
	if n, err := w.Sweep(ctx, time.Minute); n != 0 || err != nil {
		t.Fatalf("too early: swept = %d, err = %v, want 0", n, err)
	}
	*clock = clock.Add(2 * time.Minute)
	if n, err := w.Sweep(ctx, time.Minute); n != 1 || err != nil {
		t.Fatalf("after the timeout: swept = %d, err = %v, want 1", n, err)
	}
	if ok, err := w.RunOne(ctx); !ok || err != nil {
		t.Fatalf("after the sweep: ran = %v, err = %v, want a job and no error", ok, err)
	}
}

// A job that takes its worker down every time is dead after the last attempt, like any other.
func TestSweptJobDies(t *testing.T) {
	w, db, clock, _ := queue(t)
	ctx := context.Background()

	for range MaxAttempts {
		if _, err := w.claim(ctx); err != nil {
			t.Fatal(err)
		}
		*clock = clock.Add(2 * time.Minute)
		if n, err := w.Sweep(ctx, time.Minute); n != 1 || err != nil {
			t.Fatalf("swept = %d, err = %v, want 1", n, err)
		}
	}
	var status string
	err := db.QueryRow(ctx, `select status from jobs`).Scan(&status)
	if err != nil || status != "dead" {
		t.Fatalf("status = %q, err = %v, want a dead job", status, err)
	}
}
