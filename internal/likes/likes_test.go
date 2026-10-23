package likes_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/fiston-user/murmur/internal/jobs"
	"github.com/fiston-user/murmur/internal/likes"
	"github.com/fiston-user/murmur/internal/notify"
	"github.com/fiston-user/murmur/internal/platform/replica"
	"github.com/fiston-user/murmur/internal/platform/testdb"
	"github.com/fiston-user/murmur/internal/posts"
	"github.com/fiston-user/murmur/internal/timeline"
	"github.com/fiston-user/murmur/internal/users"
	"github.com/jackc/pgx/v5/pgxpool"
)

// app is a real server on the test database, and a worker for the jobs it queues.
type app struct {
	t   *testing.T
	url string
	db  *pgxpool.Pool
	n   *notify.Handler
	w   *jobs.Worker
}

func newApp(t *testing.T) *app {
	db := testdb.New(t)
	reads := replica.New(db, nil, nil) // no replica: every read goes to db
	u, p, l := users.New(db, nil), posts.New(db, reads), likes.New(db)
	n := notify.New(db, testdb.Redis(t))
	w := jobs.NewWorker(db)
	w.Handle(notify.Like, n.RunLike)
	// A post queues a fan-out job as well. Nobody follows anybody here, so there's nothing to do.
	w.Handle(timeline.FanOut, func(context.Context, []byte) error { return nil })
	mux := http.NewServeMux()
	mux.HandleFunc("POST /users", u.Create)
	mux.HandleFunc("POST /posts", u.Auth(p.Create))
	mux.HandleFunc("POST /posts/{id}/like", u.Auth(l.Like))
	mux.HandleFunc("DELETE /posts/{id}/like", u.Auth(l.Unlike))
	mux.HandleFunc("GET /notifications", u.Auth(n.List))
	mux.HandleFunc("GET /notifications/unread", u.Auth(n.Unread))
	mux.HandleFunc("POST /notifications/read", u.Auth(n.Read))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &app{t: t, url: srv.URL, db: db, n: n, w: w}
}

// do sends one request and returns its status. A JSON answer is decoded into out (unless out is nil).
func (a *app) do(method, path, token, body string, out any) int {
	req, err := http.NewRequest(method, a.url+path, strings.NewReader(body))
	if err != nil {
		a.t.Error(err)
		return 0
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Error(err)
		return 0
	}
	defer res.Body.Close()
	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			a.t.Errorf("%s %s: %v", method, path, err)
		}
	}
	return res.StatusCode
}

// must is do, for a request that has to answer with the status we want.
func (a *app) must(want int, method, path, token, body string, out any) {
	a.t.Helper()
	if got := a.do(method, path, token, body, out); got != want {
		a.t.Fatalf("%s %s: status %d, want %d", method, path, got, want)
	}
}

// signUp creates a user and returns their token.
func (a *app) signUp(handle string) string {
	a.t.Helper()
	var out struct {
		Token string `json:"token"`
	}
	a.must(http.StatusCreated, "POST", "/users", "", fmt.Sprintf(`{"handle": %q}`, handle), &out)
	return out.Token
}

// settle does what the worker does, right now: it folds the deltas, and it runs every job.
func (a *app) settle() {
	a.t.Helper()
	ctx := context.Background()
	if _, err := likes.Fold(ctx, a.db); err != nil {
		a.t.Fatal(err)
	}
	for {
		ok, err := a.w.RunOne(ctx)
		if err != nil {
			a.t.Fatal(err)
		}
		if !ok {
			return
		}
	}
}

// count runs a query that returns one number.
func (a *app) count(sql string) int {
	a.t.Helper()
	var n int
	if err := a.db.QueryRow(context.Background(), sql).Scan(&n); err != nil {
		a.t.Fatal(err)
	}
	return n
}

// A like is a fact, and a fact is true once: liking twice counts once.
func TestLikeTwice(t *testing.T) {
	a := newApp(t)
	ann, bob := a.signUp("ann"), a.signUp("bob")
	a.must(http.StatusCreated, "POST", "/posts", ann, `{"body": "hello"}`, nil)

	a.must(http.StatusNoContent, "POST", "/posts/1/like", bob, "", nil)
	a.must(http.StatusNoContent, "POST", "/posts/1/like", bob, "", nil)
	a.settle()
	if n := a.count(`select likes_count from posts where id = 1`); n != 1 {
		t.Fatalf("two likes by one user: the count is %d, want 1", n)
	}
	a.must(http.StatusNoContent, "DELETE", "/posts/1/like", bob, "", nil)
	a.must(http.StatusNoContent, "DELETE", "/posts/1/like", bob, "", nil)
	a.settle()
	if n := a.count(`select likes_count from posts where id = 1`); n != 0 {
		t.Fatalf("after the unlike: the count is %d, want 0", n)
	}
	a.must(http.StatusNotFound, "POST", "/posts/2/like", bob, "", nil)
}

// A hundred people like one post at the same moment, twice each, while two
// folders fold flat out. Nobody is lost, and nobody is counted twice.
func TestLikeCrowd(t *testing.T) {
	a := newApp(t)
	a.must(http.StatusCreated, "POST", "/posts", a.signUp("ann"), `{"body": "hello"}`, nil)
	var fans []string
	for i := range 100 {
		fans = append(fans, a.signUp(fmt.Sprintf("fan%d", i)))
	}

	var done atomic.Bool
	var crowd, folders sync.WaitGroup
	for range 2 {
		folders.Go(func() {
			for !done.Load() {
				if _, err := likes.Fold(context.Background(), a.db); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	start := make(chan struct{})
	for _, fan := range fans {
		crowd.Go(func() {
			<-start
			for range 2 {
				if got := a.do("POST", "/posts/1/like", fan, "", nil); got != http.StatusNoContent {
					t.Errorf("a like answered %d", got)
				}
			}
		})
	}
	close(start)
	crowd.Wait()
	done.Store(true)
	folders.Wait()
	a.settle()

	if n := a.count(`select count(*) from likes`); n != 100 {
		t.Fatalf("%d likes, want 100", n)
	}
	if n := a.count(`select likes_count from posts where id = 1`); n != 100 {
		t.Fatalf("the count is %d, want 100", n)
	}
	if n := a.count(`select count(*) from like_deltas`); n != 0 {
		t.Fatalf("%d deltas are left, want none", n)
	}
}

// A job can run twice. The second run of a like's job writes nothing,
// and it doesn't fail. And a like for your own post tells nobody.
func TestLikeJobTwice(t *testing.T) {
	a := newApp(t)
	ann, bob := a.signUp("ann"), a.signUp("bob")
	a.must(http.StatusCreated, "POST", "/posts", ann, `{"body": "hello"}`, nil)
	a.must(http.StatusNoContent, "POST", "/posts/1/like", bob, "", nil)
	a.must(http.StatusNoContent, "POST", "/posts/1/like", ann, "", nil)
	a.settle()

	// The queue ran bob's job once. Now it runs a second time.
	if err := a.n.RunLike(context.Background(), []byte(`{"liker": 2, "post": 1}`)); err != nil {
		t.Fatalf("the second run failed: %v", err)
	}
	if n := a.count(`select count(*) from notifications`); n != 1 {
		t.Fatalf("%d notifications, want 1", n)
	}
}

// crowded gives ann a post, and n notifications about it, from n users made in SQL.
func (a *app) crowded(n int) (ann string) {
	a.t.Helper()
	ann = a.signUp("ann")
	a.must(http.StatusCreated, "POST", "/posts", ann, `{"body": "hello"}`, nil)
	_, err := a.db.Exec(context.Background(), `
		with fans as (
			insert into users (handle, token_hash)
			select 'fan' || i, sha256(i::text::bytea) from generate_series(1, $1::int) i
			returning id
		)
		insert into notifications (user_id, kind, actor_id, post_id)
		select 1, 'like', id, 1 from fans order by id`, n)
	if err != nil {
		a.t.Fatal(err)
	}
	return ann
}

func TestUnread(t *testing.T) {
	a := newApp(t)
	ann := a.crowded(150)
	unread := func() int {
		t.Helper()
		var out struct {
			Unread int `json:"unread"`
		}
		a.must(http.StatusOK, "GET", "/notifications/unread", ann, "", &out)
		return out.Unread
	}
	steps := []struct {
		upTo int // ann says she has seen everything up to this id
		want int // and this is the unread count after it
	}{
		{0, notify.MaxUnread}, // 150 are unread, and the count stops at a hundred
		{100, 50},
		{148, 2},
		{20, 2},  // the marker never moves back
		{999, 0}, // an id she doesn't have lands on her newest one
	}
	for i, s := range steps {
		if s.upTo > 0 {
			seen := fmt.Sprintf(`{"up_to": %d}`, s.upTo)
			a.must(http.StatusNoContent, "POST", "/notifications/read", ann, seen, nil)
		}
		if got := unread(); got != s.want {
			t.Fatalf("step %d: unread = %d, want %d", i+1, got, s.want)
		}
	}
}

func TestNotificationPages(t *testing.T) {
	a := newApp(t)
	ann := a.crowded(5)
	a.must(http.StatusNoContent, "POST", "/notifications/read", ann, `{"up_to": 3}`, nil)

	var got, unread []int64
	path := "/notifications?limit=2"
	for {
		var page notify.Page
		a.must(http.StatusOK, "GET", path, ann, "", &page)
		for _, n := range page.Notifications {
			got = append(got, n.ID)
			if n.Unread {
				unread = append(unread, n.ID)
			}
		}
		if page.NextCursor == nil {
			break
		}
		path = fmt.Sprintf("/notifications?limit=2&cursor=%d", *page.NextCursor)
	}
	if want := []int64{5, 4, 3, 2, 1}; !slices.Equal(got, want) {
		t.Fatalf("notification ids = %v, want %v", got, want)
	}
	if want := []int64{5, 4}; !slices.Equal(unread, want) {
		t.Fatalf("unread ids = %v, want %v", unread, want)
	}
}
