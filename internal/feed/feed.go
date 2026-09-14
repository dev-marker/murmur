// Package feed is the home feed: the posts of the people you follow.
package feed

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/fiston-user/murmur/internal/platform/httpx"
	"github.com/fiston-user/murmur/internal/users"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Post is one post in a feed, with its author's handle.
type Post struct {
	ID        int64     `json:"id"`
	Handle    string    `json:"handle"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// Page is one page of a feed. NextCursor is null on the last page.
type Page struct {
	NextCursor *int64 `json:"next_cursor"`
	Posts      []Post `json:"posts"`
}

type Handler struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// List returns a page of the caller's home feed, newest first.
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
			httpx.Error(w, http.StatusBadRequest, "cursor must be a post id")
			return
		}
		cursor = id
	}
	rows, _ := h.db.Query(r.Context(), `
		select p.id, u.handle, p.body, p.created_at
		from follows f
		cross join lateral (
			select id, user_id, body, created_at
			from posts
			where user_id = f.followee_id and id < $2
			order by id desc
			limit $3
		) p
		join users u on u.id = p.user_id
		where f.follower_id = $1
		order by p.id desc
		limit $3`, users.ID(r.Context()), cursor, limit+1)
	posts, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Post])
	if err != nil {
		httpx.ServerError(w, r, err)
		return
	}
	page := Page{Posts: posts}
	if len(posts) > limit {
		page.Posts = posts[:limit]
		page.NextCursor = &posts[limit-1].ID
	}
	httpx.JSON(w, http.StatusOK, page)
}
