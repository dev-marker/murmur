package live

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/fiston-user/murmur/internal/platform/testdb"
)

func like(id int64) Event {
	return Event{ID: id, Kind: "like", Data: []byte(`{"actor":"bob","post_id":7}`)}
}

// open is how many streams the hub holds for a user.
func (h *Hub) open(user int64) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.streams[user])
}

// An event goes to every stream its user has open, and to nobody else.
func TestDeliver(t *testing.T) {
	h := NewHub()
	phone, laptop, other := h.Register(1), h.Register(1), h.Register(2)
	h.Deliver(1, like(5))
	for _, c := range []chan Event{phone, laptop} {
		if got := <-c; got.ID != 5 {
			t.Fatalf("got event %d, want 5", got.ID)
		}
	}
	if len(other) != 0 {
		t.Fatal("user 2 got an event that was for user 1")
	}
	h.Unregister(1, phone)
	h.Unregister(1, laptop)
	h.Deliver(1, like(6)) // nobody is there, and that's fine
	if h.open(1) != 0 || h.open(2) != 1 {
		t.Fatalf("streams left: %d for user 1, %d for user 2, want 0 and 1", h.open(1), h.open(2))
	}
}

// A stream that nobody reads fills up, and then the hub lets go of it.
// The stream next to it, which is read, never notices.
func TestSlowStreamIsDropped(t *testing.T) {
	h := NewHub()
	slow, fast := h.Register(1), h.Register(1)
	for i := range int64(buffer + 1) {
		h.Deliver(1, like(i+1))
		if got := <-fast; got.ID != i+1 {
			t.Fatalf("the fast stream got event %d, want %d", got.ID, i+1)
		}
	}
	n := 0
	for range slow { // the channel was closed, so this loop ends
		n++
	}
	if n != buffer || h.open(1) != 1 || h.dropped != 1 {
		t.Fatalf("the slow stream held %d events, %d streams are left, %d dropped: want %d, 1 and 1",
			n, h.open(1), h.dropped, buffer)
	}
	h.Unregister(1, slow) // its handler still says goodbye, and nothing breaks
}

// Streams come and go while events are delivered: the race detector watches the hub.
func TestHubCrowd(t *testing.T) {
	h := NewHub()
	var crowd sync.WaitGroup
	for user := range int64(50) {
		crowd.Go(func() {
			for range 100 {
				c := h.Register(user)
				h.Deliver(user, like(1))
				h.Deliver((user+1)%50, like(2))
				h.Unregister(user, c)
			}
		})
	}
	crowd.Wait()
	if len(h.streams) != 0 {
		t.Fatalf("%d users still have streams, want none", len(h.streams))
	}
}

// server is a real HTTP server with Stream behind it. left gets a value every time
// a stream's handler returns.
func server(t *testing.T, h *Handler) (url string, left chan struct{}) {
	left = make(chan struct{}, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.Stream(w, r)
		left <- struct{}{}
	}))
	t.Cleanup(srv.Close)
	return srv.URL, left
}

// connect opens a stream and reads up to the first comment, so the stream is registered.
func connect(t *testing.T, ctx context.Context, url, lastID string) *bufio.Scanner {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	if got := res.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type is %q", got)
	}
	lines := bufio.NewScanner(res.Body)
	next(t, lines, ": connected", "")
	return lines
}

// next reads the lines we expect, one after the other.
func next(t *testing.T, lines *bufio.Scanner, want ...string) {
	t.Helper()
	for _, w := range want {
		if !lines.Scan() || lines.Text() != w {
			t.Fatalf("the stream said %q, want %q", lines.Text(), w)
		}
	}
}

// An event travels down the stream as lines of text, and a client that leaves gives its place back.
func TestStream(t *testing.T) {
	hub := NewHub()
	h := &Handler{Hub: hub, User: func(context.Context) int64 { return 1 }}
	url, left := server(t, h)
	ctx, hangUp := context.WithCancel(context.Background())
	lines := connect(t, ctx, url, "")

	hub.Deliver(1, like(5))
	hub.Deliver(1, Event{Kind: "post", Data: []byte(`{"post_id":7}`)})
	next(t, lines, "id: 5", "event: like", `data: {"actor":"bob","post_id":7}`, "",
		"event: post", `data: {"post_id":7}`, "")

	hangUp()
	<-left
	if hub.open(1) != 0 {
		t.Fatal("the client left, and its stream is still in the hub")
	}
}

// A client that says what it saw last gets what it missed first, and nothing twice.
func TestStreamCatchesUp(t *testing.T) {
	hub := NewHub()
	h := &Handler{Hub: hub, User: func(context.Context) int64 { return 1 }}
	h.Missed = func(_ context.Context, user, after int64) ([]Event, error) {
		if user != 1 || after != 5 {
			return nil, fmt.Errorf("asked for user %d after %d", user, after)
		}
		hub.Deliver(1, like(7)) // event 7 happens right now: it's in the answer, and in the buffer
		return []Event{like(6), like(7)}, nil
	}
	url, _ := server(t, h)
	lines := connect(t, context.Background(), url, "5")

	hub.Deliver(1, like(8))
	var ids []string
	for len(ids) < 3 && lines.Scan() {
		if id, ok := strings.CutPrefix(lines.Text(), "id: "); ok {
			ids = append(ids, id)
		}
	}
	if got := strings.Join(ids, " "); got != "6 7 8" {
		t.Fatalf("the stream sent events %s, want 6 7 8", got)
	}
}

// Two servers, one Redis: what is published reaches the streams on both.
// And a hub that starts to listen lets go of whoever was connected before.
func TestSubscribe(t *testing.T) {
	rdb := testdb.Redis(t)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	a, b := NewHub(), NewHub()
	early := []chan Event{a.Register(1), b.Register(1)}
	go a.Subscribe(ctx, rdb)
	go b.Subscribe(ctx, rdb)
	for _, c := range early {
		if _, open := <-c; open {
			t.Fatal("a stream from before the hub listened got an event")
		}
	}

	onA, onB := a.Register(1), b.Register(1)
	if err := Publish(ctx, rdb, 1, like(5)); err != nil {
		t.Fatal(err)
	}
	for _, c := range []chan Event{onA, onB} {
		got := <-c
		if got.ID != 5 || got.Kind != "like" || string(got.Data) != string(like(5).Data) {
			t.Fatalf("got %+v", got)
		}
	}
}
