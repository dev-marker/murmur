// Package live pushes news to the clients that are connected right now.
package live

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/fiston-user/murmur/internal/platform/httpx"
)

// beat is how often a quiet stream says something.
const beat = 15 * time.Second

// wait is how long one write to a client may take.
const wait = 10 * time.Second

// A Handler serves GET /live: one user's events, as they happen.
type Handler struct {
	Done   <-chan struct{}             // closed when this server shuts down
	Hub    *Hub                        // the streams that are open on this server
	User   func(context.Context) int64 // who a request is from
	Missed Missed                      // what a client missed while it was away
}

// Missed returns a user's events with an id above after, oldest first.
type Missed func(ctx context.Context, user, after int64) ([]Event, error)

// Stream answers with text/event-stream, and it doesn't return for as long as
// the client listens.
func (h *Handler) Stream(w http.ResponseWriter, r *http.Request) {
	user := h.User(r.Context())
	events := h.Hub.Register(user)
	defer h.Hub.Unregister(user, events)
	after, err := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	var old []Event
	if err == nil {
		if old, err = h.Missed(r.Context(), user, after); err != nil {
			httpx.ServerError(w, r, err)
			return
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	send := func(text string) error {
		rc.SetWriteDeadline(time.Now().Add(wait))
		fmt.Fprint(w, text)
		return rc.Flush()
	}
	send(": connected\n\n")
	var seen int64
	for _, e := range old {
		if send(e.text()) != nil {
			return
		}
		seen = e.ID
	}
	tick := time.NewTicker(beat)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-h.Done:
			return
		case <-tick.C:
			if send(": still here\n\n") != nil {
				return
			}
		case e, open := <-events:
			if !open {
				return
			}
			if e.ID != 0 && e.ID <= seen {
				continue
			}
			if send(e.text()) != nil {
				return
			}
		}
	}
}

func (e Event) text() string {
	text := fmt.Sprintf("event: %s\ndata: %s\n\n", e.Kind, e.Data)
	if e.ID != 0 {
		text = fmt.Sprintf("id: %d\n", e.ID) + text
	}
	return text
}
