// Package feed is the home feed: the posts of the people you follow.
package feed

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/fiston-user/murmur/internal/platform/httpx"
	"github.com/fiston-user/murmur/internal/platform/replica"
	"github.com/fiston-user/murmur/internal/timeline"
	"github.com/fiston-user/murmur/internal/users"
	"github.com/jackc/pgx/v5"
)

// Post is one post in a feed, with its author's handle.
type Post struct {
	ID        int64     `json:"id"`
	Handle    string    `json:"handle"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	Likes     int       `json:"likes"`
}

// Page is one page of a feed. NextCursor is null on the last page.
type Page struct {
	NextCursor *int64 `json:"next_cursor"`
	Posts      []Post `json:"posts"`
}

type Handler struct {
	reads *replica.Reads
	tl    *timeline.Store
}

func New(reads *replica.Reads, tl *timeline.Store) *Handler {
	return &Handler{reads: reads, tl: tl}
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
	db := h.reads.For(r.Context(), users.ID(r.Context()))
	ids, err := h.tl.Page(r.Context(), db, users.ID(r.Context()), cursor, limit+1)
	if err != nil {
		httpx.ServerError(w, r, err)
		return
	}
	rows, _ := db.Query(r.Context(), `
		select p.id, u.handle, p.body, p.created_at, p.likes_count
		from posts p
		join users u on u.id = p.user_id
		where p.id = any($1)
		order by p.id desc`, ids)
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
