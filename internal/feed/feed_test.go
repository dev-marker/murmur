package feed_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/fiston-user/murmur/internal/feed"
	"github.com/fiston-user/murmur/internal/jobs"
	"github.com/fiston-user/murmur/internal/platform/replica"
	"github.com/fiston-user/murmur/internal/platform/testdb"
	"github.com/fiston-user/murmur/internal/posts"
	"github.com/fiston-user/murmur/internal/timeline"
	"github.com/fiston-user/murmur/internal/users"
	"github.com/redis/go-redis/v9"
)

// client talks to a real server on a real database, for one test.
type client struct {
	t   *testing.T
	url string
	rdb *redis.Client
	w   *jobs.Worker
	tl  *timeline.Store
}

func newClient(t *testing.T) *client {
	db, rdb := testdb.New(t), testdb.Redis(t)
	tl := timeline.New(db, rdb, 2)
	reads := replica.New(db, nil, rdb) // no replica: every read goes to db
	u, p, f := users.New(db, tl), posts.New(db, reads), feed.New(reads, tl)
	w := jobs.NewWorker(db)
	w.Handle(timeline.FanOut, tl.RunFanOut)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /users", u.Create)
	mux.HandleFunc("POST /posts", u.Auth(p.Create))
	mux.HandleFunc("POST /users/{handle}/follow", u.Auth(u.Follow))
	mux.HandleFunc("GET /feed", u.Auth(f.List))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &client{t: t, url: srv.URL, rdb: rdb, w: w, tl: tl}
}

// call sends one request, checks the status, and decodes a JSON answer into out (unless out is nil).
func (c *client) call(method, path, token, body string, want int, out any) {
	c.t.Helper()
	req, err := http.NewRequest(method, c.url+path, strings.NewReader(body))
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != want {
		c.t.Fatalf("%s %s: status %d, want %d", method, path, res.StatusCode, want)
	}
	c.work() // run what this request queued
	if out == nil {
		return
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
}

// work runs the queue until it's empty, like the worker does, minus the sleep.
func (c *client) work() {
	c.t.Helper()
	for {
		ok, err := c.w.RunOne(context.Background())
		if err != nil {
			c.t.Fatal(err)
		}
		if !ok {
			return
		}
	}
}

// signUp creates a user and returns their token.
func (c *client) signUp(handle string) string {
	c.t.Helper()
	var out struct {
		Token string `json:"token"`
	}
	c.call("POST", "/users", "", fmt.Sprintf(`{"handle": %q}`, handle), http.StatusCreated, &out)
	return out.Token
}

func TestFeedPaging(t *testing.T) {
	c := newClient(t)
	alice, bob, carol := c.signUp("alice"), c.signUp("bob"), c.signUp("carol")
	c.call("POST", "/users/alice/follow", carol, "", http.StatusNoContent, nil)
	c.call("POST", "/users/bob/follow", carol, "", http.StatusNoContent, nil)
	for i, token := range []string{alice, bob, alice, bob, carol} {
		c.call("POST", "/posts", token, fmt.Sprintf(`{"body": "post %d"}`, i+1), http.StatusCreated, nil)
	}

	var got []int64
	path := "/feed?limit=2"
	for {
		var page feed.Page
		c.call("GET", path, carol, "", http.StatusOK, &page)
		for _, p := range page.Posts {
			got = append(got, p.ID)
		}
		if page.NextCursor == nil {
			break
		}
		path = fmt.Sprintf("/feed?limit=2&cursor=%d", *page.NextCursor)
		c.call("POST", "/posts", bob, `{"body": "while you were reading"}`, http.StatusCreated, nil)
	}
	if want := []int64{4, 3, 2, 1}; !slices.Equal(got, want) {
		t.Fatalf("feed ids = %v, want %v", got, want)
	}
}

func TestTimeline(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	alice, bob := c.signUp("alice"), c.signUp("bob")
	c.call("POST", "/users/alice/follow", bob, "", http.StatusNoContent, nil)
	post := func(token string) {
		c.call("POST", "/posts", token, `{"body": "hello"}`, http.StatusCreated, nil)
	}
	post(alice)
	post(alice)

	// Bob has never read his feed, so those two posts must not start a timeline for him.
	if n := c.rdb.Exists(ctx, "timeline:2").Val(); n != 0 {
		t.Fatal("a post started a timeline for a reader who has none")
	}
	// His first read builds it, with both posts.
	var page feed.Page
	c.call("GET", "/feed", bob, "", http.StatusOK, &page)
	if len(page.Posts) != 2 {
		t.Fatalf("first read: %d posts, want 2", len(page.Posts))
	}
	// From now on, a new post reaches it. The zero is the marker.
	post(alice)
	got := c.rdb.ZRange(ctx, "timeline:2", 0, -1).Val()
	if want := []string{"0", "1", "2", "3"}; !slices.Equal(got, want) {
		t.Fatalf("cached timeline = %v, want %v", got, want)
	}
	// Reads come from Redis. To prove it, we plant one of bob's own posts.
	post(bob)
	c.rdb.ZAdd(ctx, "timeline:2", redis.Z{Score: 4, Member: 4})
	c.call("GET", "/feed?limit=1", bob, "", http.StatusOK, &page)
	if len(page.Posts) != 1 || page.Posts[0].ID != 4 {
		t.Fatalf("page = %+v, want post 4: the read didn't come from Redis", page.Posts)
	}
}

func TestBigAccount(t *testing.T) {
	c := newClient(t) // in these tests, two followers make an account big
	ctx := context.Background()
	alice, bob, carol, dave := c.signUp("alice"), c.signUp("bob"), c.signUp("carol"), c.signUp("dave")
	c.call("POST", "/users/alice/follow", bob, "", http.StatusNoContent, nil)
	c.call("POST", "/users/alice/follow", carol, "", http.StatusNoContent, nil)
	// Bob follows dave twice. That must count once, or dave would be big too.
	c.call("POST", "/users/dave/follow", bob, "", http.StatusNoContent, nil)
	c.call("POST", "/users/dave/follow", bob, "", http.StatusNoContent, nil)
	// Bob reads once, so he has a timeline for posts to land in.
	c.call("GET", "/feed", bob, "", http.StatusOK, nil)
	for _, token := range []string{dave, alice, dave, alice} {
		c.call("POST", "/posts", token, `{"body": "hello"}`, http.StatusCreated, nil)
	}

	// Dave is small, so his posts were pushed. Alice is big, so hers were not.
	got := c.rdb.ZRange(ctx, "timeline:2", 0, -1).Val()
	if want := []string{"0", "1", "3"}; !slices.Equal(got, want) {
		t.Fatalf("bob's timeline = %v, want %v", got, want)
	}
	// And still, alice's newest post is on top of bob's feed.
	var page feed.Page
	c.call("GET", "/feed?limit=1", bob, "", http.StatusOK, &page)
	if len(page.Posts) != 1 || page.Posts[0].ID != 4 {
		t.Fatalf("page = %+v, want post 4, pulled from a big account", page.Posts)
	}
}

// A job can run twice. The second fan-out of a post must change nothing.
func TestFanOutTwice(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	alice, bob := c.signUp("alice"), c.signUp("bob")
	c.call("POST", "/users/alice/follow", bob, "", http.StatusNoContent, nil)
	c.call("GET", "/feed", bob, "", http.StatusOK, nil) // bob has a timeline now
	c.call("POST", "/posts", alice, `{"body": "hello"}`, http.StatusCreated, nil)

	// The queue ran this job once. Now it runs a second time.
	if err := c.tl.RunFanOut(ctx, []byte(`{"author": 1, "post": 1}`)); err != nil {
		t.Fatal(err)
	}
	got := c.rdb.ZRange(ctx, "timeline:2", 0, -1).Val()
	if want := []string{"0", "1"}; !slices.Equal(got, want) {
		t.Fatalf("bob's timeline = %v, want %v", got, want)
	}
}
