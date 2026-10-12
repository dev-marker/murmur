package live

import (
	"context"
	"encoding/json"
	"log/slog"
	"runtime"
	"sync"
	"time"
)

// An Event is one piece of news for one user. An ID is something a client can
// come back for after a break. Zero means there is nothing to come back for.
type Event struct {
	ID   int64           `json:"id,omitempty"`
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

// buffer is how many events may wait for one stream.
const buffer = 64

// A Hub knows the streams that are open on this server, by user.
type Hub struct {
	mu      sync.Mutex
	streams map[int64]map[chan Event]bool
	dropped int
}

func NewHub() *Hub {
	return &Hub{streams: map[int64]map[chan Event]bool{}}
}

func (h *Hub) Register(user int64) chan Event {
	c := make(chan Event, buffer)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.streams[user] == nil {
		h.streams[user] = map[chan Event]bool{}
	}
	h.streams[user][c] = true
	return c
}

func (h *Hub) Unregister(user int64, c chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.streams[user], c)
	if len(h.streams[user]) == 0 {
		delete(h.streams, user)
	}
}

// Deliver hands e to every stream of user. It never waits.
func (h *Hub) Deliver(user int64, e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.streams[user] {
		select {
		case c <- e:
		default:
			close(c)
			delete(h.streams[user], c)
			h.dropped++
		}
	}
}

// Reset drops every stream. Their clients come back, and catch up.
func (h *Hub) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for user, open := range h.streams {
		for c := range open {
			close(c)
		}
		delete(h.streams, user)
	}
}

// Report logs how many streams are open on this server and what the whole process
// uses for them, once at the start and then whenever the numbers have changed.
func (h *Hub) Report(ctx context.Context, every time.Duration) {
	tick := time.NewTicker(every)
	defer tick.Stop()
	was := [2]int{-1, 0} // so the first look is always logged
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		h.mu.Lock()
		now := [2]int{0, h.dropped}
		for _, open := range h.streams {
			now[0] += len(open)
		}
		h.mu.Unlock()
		if now == was {
			continue
		}
		was = now
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		mb := (m.HeapInuse + m.StackInuse) >> 20
		slog.Info("hub", "open", now[0], "dropped", now[1], "goroutines", runtime.NumGoroutine(), "mb", mb)
	}
}
