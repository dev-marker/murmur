package live

import (
	"encoding/json"
	"sync"
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

// Open is how many streams are open on this server, right now.
func (h *Hub) Open() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, open := range h.streams {
		n += len(open)
	}
	return n
}

// Dropped is how many streams this server has let go of, because they fell behind.
func (h *Hub) Dropped() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.dropped
}
