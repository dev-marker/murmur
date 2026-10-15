package metrics

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fiston-user/murmur/internal/jobs"
	"github.com/fiston-user/murmur/internal/platform/testdb"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// measured is a small API behind Wrap, the way cmd/api builds the real one.
func measured(t *testing.T) (*HTTP, *httptest.Server) {
	m := NewHTTP(prometheus.NewRegistry())
	mux := http.NewServeMux()
	mux.HandleFunc("POST /posts/{id}/like", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /feed", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "[]") // no WriteHeader: that's a 200
	})
	mux.HandleFunc("GET /live", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, ": connected\n\n")
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("the stream can't flush: %v", err)
			return
		}
		<-r.Context().Done()
	})
	srv := httptest.NewServer(m.Wrap(mux.ServeHTTP))
	t.Cleanup(srv.Close)
	return m, srv
}

// The route label is the pattern, never the path: a million posts are one series.
func TestRouteIsThePattern(t *testing.T) {
	m, srv := measured(t)
	for _, post := range []string{"7", "8", "2000001"} {
		res, err := http.Post(srv.URL+"/posts/"+post+"/like", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
	}
	for _, path := range []string{"/feed", "/nothing/here"} {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
	}
	for _, want := range []struct {
		route, status string
		n             float64
	}{
		{"POST /posts/{id}/like", "204", 3},
		{"GET /feed", "200", 1},
		{"other", "404", 1},
	} {
		got := testutil.ToFloat64(m.requests.WithLabelValues(want.route, want.status))
		if got != want.n {
			t.Errorf("%s %s: counted %v requests, want %v", want.route, want.status, got, want.n)
		}
	}
	if n := testutil.CollectAndCount(m.requests); n != 3 {
		t.Errorf("the counter has %d series, want 3", n)
	}
	// one histogram for every route that answered
	if n := testutil.CollectAndCount(m.duration); n != 3 {
		t.Errorf("%d routes were timed, want 3", n)
	}
}

// A stream goes through the middleware untouched: its first line arrives while
// the request is still open. And a stream is counted, but it is not timed.
func TestStreamStillFlushes(t *testing.T) {
	m, srv := measured(t)
	ctx, hangUp := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/live", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	line, _ := bufio.NewReader(res.Body).ReadString('\n')
	if strings.TrimSpace(line) != ": connected" {
		t.Fatalf("the stream said %q, want the connected comment", line)
	}
	hangUp()
	srv.Close() // it waits until the handler has returned
	if got := testutil.ToFloat64(m.requests.WithLabelValues("GET /live", "200")); got != 1 {
		t.Errorf("counted %v streams, want 1", got)
	}
	if n := testutil.CollectAndCount(m.duration); n != 0 {
		t.Errorf("%d routes were timed, want none: a stream is not a slow request", n)
	}
}

// The queue's numbers come from Postgres at the moment somebody asks.
func TestQueueIsReadWhenAsked(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	q := NewQueue(db)
	const waiting = `
		# HELP murmur_queue_jobs Jobs in the queue.
		# TYPE murmur_queue_jobs gauge
		murmur_queue_jobs{status="dead"} 0
		murmur_queue_jobs{status="running"} 0
		murmur_queue_jobs{status="waiting"} %d
		# HELP murmur_like_deltas Likes not yet in a post's count.
		# TYPE murmur_like_deltas gauge
		murmur_like_deltas %d
	`
	check := func(jobs, deltas int) {
		t.Helper()
		want := strings.NewReader(fmt.Sprintf(waiting, jobs, deltas))
		err := testutil.CollectAndCompare(q, want, "murmur_queue_jobs", "murmur_like_deltas")
		if err != nil {
			t.Fatal(err)
		}
	}
	check(0, 0)
	for range 3 {
		if err := jobs.Enqueue(ctx, db, "like", map[string]int{"post": 1}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `insert into like_deltas (post_id, delta) values (1, 1), (1, 1)`); err != nil {
		t.Fatal(err)
	}
	check(3, 2)
}
