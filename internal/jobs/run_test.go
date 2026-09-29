package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/fiston-user/murmur/internal/platform/testdb"
)

// A job that waited in the queue leaves its wait in the tally, once.
func TestWaitIsCounted(t *testing.T) {
	w, _, clock, fails := queue(t)
	ctx := context.Background()
	*fails = false

	*clock = clock.Add(90 * time.Second)
	if ok, err := w.RunOne(ctx); !ok || err != nil {
		t.Fatalf("ran = %v, err = %v, want a job and no error", ok, err)
	}
	waits := w.done.take()
	if len(waits) != 1 || waits[0].Round(time.Second) != 90*time.Second {
		t.Fatalf("waits = %v, want one wait of 1m30s", waits)
	}
	if again := w.done.take(); len(again) != 0 {
		t.Fatalf("a second take = %v, want nothing", again)
	}
}

// Run is cancelled while a job is in hand. It returns only when that job is finished.
func TestRunFinishesTheJobInHand(t *testing.T) {
	db := testdb.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := Enqueue(ctx, db, "slow", 1); err != nil {
		t.Fatal(err)
	}
	started, finish := make(chan struct{}), make(chan struct{})
	w := NewWorker(db)
	w.Handle("slow", func(context.Context, []byte) error {
		close(started)
		<-finish
		return nil
	})
	stopped := make(chan struct{})
	go func() {
		w.Run(ctx, 1, time.Millisecond, time.Minute, time.Minute)
		close(stopped)
	}()

	<-started
	cancel()
	select {
	case <-stopped:
		t.Fatal("Run returned with a job in hand")
	default:
	}
	close(finish)
	<-stopped

	var left int
	err := db.QueryRow(context.Background(), `select count(*) from jobs`).Scan(&left)
	if err != nil || left != 0 {
		t.Fatalf("jobs left = %d, err = %v, want an empty queue", left, err)
	}
}

// A job that doesn't finish within the grace period is cut off, and it stays running.
// To the queue, its worker has gone away, and a sweep will find the job.
func TestRunCutsOffAfterGrace(t *testing.T) {
	db := testdb.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := Enqueue(ctx, db, "endless", 1); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	w := NewWorker(db)
	w.Handle("endless", func(ctx context.Context, _ []byte) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	stopped := make(chan struct{})
	go func() {
		w.Run(ctx, 1, time.Millisecond, time.Minute, time.Millisecond)
		close(stopped)
	}()

	<-started
	cancel()
	<-stopped
	var status string
	err := db.QueryRow(context.Background(), `select status from jobs`).Scan(&status)
	if err != nil || status != "running" {
		t.Fatalf("status = %q, err = %v, want a job left running for the sweep", status, err)
	}
}
