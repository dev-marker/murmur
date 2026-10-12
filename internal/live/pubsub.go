package live

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/redis/go-redis/v9"
)

// channel is the one Redis channel that every API server listens to.
const channel = "live"

// message is what travels through Redis: an event, and the user it is for.
type message struct {
	User int64 `json:"user"`
	Event
}

// Publish announces e to user, on whichever servers they are connected to.
func Publish(ctx context.Context, rdb redis.Cmdable, user int64, e Event) error {
	b, err := json.Marshal(message{User: user, Event: e})
	if err != nil {
		return err
	}
	return rdb.Publish(ctx, channel, b).Err()
}

// Subscribe hands what is published to this server's streams, until ctx is cancelled.
func (h *Hub) Subscribe(ctx context.Context, rdb *redis.Client) {
	sub := rdb.Subscribe(ctx, channel)
	defer sub.Close()
	in := sub.ChannelWithSubscriptions()
	for {
		select {
		case <-ctx.Done():
			return
		case got := <-in:
			switch got := got.(type) {
			case *redis.Subscription:
				slog.Info("listening for live updates")
				h.Reset()
			case *redis.Message:
				var m message
				if err := json.Unmarshal([]byte(got.Payload), &m); err != nil {
					slog.Error("a live update could not be read", "error", err)
					continue
				}
				h.Deliver(m.User, m.Event)
			}
		}
	}
}
