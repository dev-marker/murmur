package jobs_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/fiston-user/murmur/internal/jobs"
	"github.com/fiston-user/murmur/internal/platform/testdb"
)

// Eight workers empty a queue of 200 jobs at the same time. Every job must run exactly once.
func TestEveryJobRunsOnce(t *testing.T) {
	db := testdb.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const n = 200
	for i := range n {
		if err := jobs.Enqueue(ctx, db, "count", i); err != nil {
			t.Fatal(err)
		}
	}

	var mu sync.Mutex
	ran, left := make([]int, n), n
	w := jobs.NewWorker(db)
	w.Handle("count", func(ctx context.Context, payload []byte) error {
		var i int
		if err := json.Unmarshal(payload, &i); err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		ran[i]++
		if left--; left == 0 {
			cancel()
		}
		return nil
	})

	w.Run(ctx, 8, time.Millisecond, time.Minute, time.Minute)
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
