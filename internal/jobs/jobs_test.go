package jobs_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/fiston-user/murmur/internal/jobs"
	"github.com/fiston-user/murmur/internal/platform/testdb"
)

// Eight workers empty a queue of 200 jobs at the same time. Every job must run exactly once.
func TestEveryJobRunsOnce(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	const n = 200
	for i := range n {
		if err := jobs.Enqueue(ctx, db, "count", i); err != nil {
			t.Fatal(err)
		}
	}

	var mu sync.Mutex
	ran := make([]int, n)
	w := jobs.NewWorker(db)
	w.Handle("count", func(ctx context.Context, payload []byte) error {
		var i int
		if err := json.Unmarshal(payload, &i); err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		ran[i]++
		return nil
	})

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for {
				ok, err := w.RunOne(ctx)
				if err != nil {
					t.Error(err)
				}
				if !ok {
					return
				}
			}
		})
	}
	wg.Wait()
	for i, times := range ran {
		if times != 1 {
			t.Errorf("job %d ran %d times, want 1", i, times)
		}
	}
}

// A job enqueued in a transaction that rolls back was never there.
func TestEnqueueRollsBack(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	w := jobs.NewWorker(db)

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) // after the rollback below, this one does nothing
	if err := jobs.Enqueue(ctx, tx, "count", 1); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if ok, err := w.RunOne(ctx); ok || err != nil {
		t.Fatalf("after a rollback: ran = %v, err = %v, want no job", ok, err)
	}
}

// A job whose handler fails goes back to waiting, and the next claim finds it again.
func TestFailedJobIsTriedAgain(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if err := jobs.Enqueue(ctx, db, "flaky", 1); err != nil {
		t.Fatal(err)
	}
	calls := 0
	w := jobs.NewWorker(db)
	w.Handle("flaky", func(context.Context, []byte) error {
		if calls++; calls == 1 {
			return errors.New("not this time")
		}
		return nil
	})

	if ok, err := w.RunOne(ctx); !ok || err == nil {
		t.Fatalf("first try: ran = %v, err = %v, want a job and an error", ok, err)
	}
	if ok, err := w.RunOne(ctx); !ok || err != nil {
		t.Fatalf("second try: ran = %v, err = %v, want a job and no error", ok, err)
	}
	if ok, _ := w.RunOne(ctx); ok || calls != 2 {
		t.Fatalf("after that: ran = %v, calls = %d, want an empty queue and 2 calls", ok, calls)
	}
}
