// Package notify tells people what happened to their posts.
package notify

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/fiston-user/murmur/internal/platform/httpx"
	"github.com/fiston-user/murmur/internal/users"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Like is the kind of job that tells a post's author about a like.
const Like = "like"

// LikeArgs is that job's payload.
type LikeArgs struct {
	Liker int64 `json:"liker"`
	Post  int64 `json:"post"`
}

type Handler struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// RunLike handles a Like job: it writes a notification for the post's author.
// Running it twice writes one, and liking your own post writes none.
func (h *Handler) RunLike(ctx context.Context, payload []byte) error {
	var a LikeArgs
	if err := json.Unmarshal(payload, &a); err != nil {
		return err
	}
	_, err := h.db.Exec(ctx, `
		insert into notifications (user_id, kind, actor_id, post_id)
		select p.user_id, 'like', $1, p.id
		from posts p
		where p.id = $2 and p.user_id <> $1
		on conflict do nothing`, a.Liker, a.Post)
	return err
}

// Notification is one line of a user's list.
type Notification struct {
	ID        int64     `json:"id"`
	Kind      string    `json:"kind"`
	Actor     string    `json:"actor"`
	PostID    int64     `json:"post_id"`
	CreatedAt time.Time `json:"created_at"`
	Unread    bool      `json:"unread"`
}

// Page is one page of notifications. NextCursor is null on the last page.
type Page struct {
	NextCursor    *int64         `json:"next_cursor"`
	Notifications []Notification `json:"notifications"`
}

// List returns a page of the caller's notifications, newest first.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit < 1 {
		limit = 20
	}
	limit = min(limit, 100)
	cursor := int64(math.MaxInt64)
	if s := q.Get("cursor"); s != "" {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil || id < 1 {
			httpx.Error(w, http.StatusBadRequest, "cursor must be a notification id")
			return
		}
		cursor = id
	}
	rows, _ := h.db.Query(r.Context(), `
		select n.id, n.kind, a.handle, n.post_id, n.created_at, n.id > u.seen_notification
		from notifications n
		join users a on a.id = n.actor_id
		join users u on u.id = n.user_id
		where n.user_id = $1 and n.id < $2
		order by n.id desc
		limit $3`, users.ID(r.Context()), cursor, limit+1)
	list, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Notification])
	if err != nil {
		httpx.ServerError(w, r, err)
		return
	}
	page := Page{Notifications: list}
	if len(list) > limit {
		page.Notifications = list[:limit]
		page.NextCursor = &list[limit-1].ID
	}
	httpx.JSON(w, http.StatusOK, page)
}

// MaxUnread is where the unread count stops counting. A client shows it as "99+".
const MaxUnread = 100

// Unread returns how many notifications the caller hasn't seen, up to MaxUnread.
func (h *Handler) Unread(w http.ResponseWriter, r *http.Request) {
	var n int
	err := h.db.QueryRow(r.Context(), `
		select count(*) from (
			select 1
			from notifications
			where user_id = $1 and id > (select seen_notification from users where id = $1)
			limit $2
		) unseen`, users.ID(r.Context()), MaxUnread).Scan(&n)
	if err != nil {
		httpx.ServerError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]int{"unread": n})
}

// Read marks the caller's notifications as seen, up to the id in the body.
func (h *Handler) Read(w http.ResponseWriter, r *http.Request) {
	var in struct {
		UpTo int64 `json:"up_to"`
	}
	if err := httpx.Decode(w, r, &in); err != nil || in.UpTo < 1 {
		httpx.Error(w, http.StatusBadRequest, "up_to must be a notification id")
		return
	}
	// The marker lands on a notification of their own, and it never moves back.
	_, err := h.db.Exec(r.Context(), `
		update users
		set seen_notification = greatest(seen_notification, (
			select coalesce(max(id), 0) from notifications where user_id = $1 and id <= $2))
		where id = $1`, users.ID(r.Context()), in.UpTo)
	if err != nil {
		httpx.ServerError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
