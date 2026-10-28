// Package posts writes and lists posts.
package posts

import (
	"math"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/fiston-user/murmur/internal/jobs"
	"github.com/fiston-user/murmur/internal/platform/httpx"
	"github.com/fiston-user/murmur/internal/platform/replica"
	"github.com/fiston-user/murmur/internal/timeline"
	"github.com/fiston-user/murmur/internal/users"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Post is one post, as the API returns it.
type Post struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type Handler struct {
	db    *pgxpool.Pool
	reads *replica.Reads
}

func New(db *pgxpool.Pool, reads *replica.Reads) *Handler {
	return &Handler{db: db, reads: reads}
}

// Create saves a post by the user who is calling.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Body string `json:"body"`
	}
	err := httpx.Decode(w, r, &in)
	if n := utf8.RuneCountInString(in.Body); err != nil || n < 1 || n > 280 {
		httpx.Error(w, http.StatusBadRequest, "body must be 1 to 280 characters")
		return
	}
	post := Post{UserID: users.ID(r.Context()), Body: in.Body}
	// The post and its job are saved together, or not at all.
	err = pgx.BeginFunc(r.Context(), h.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(r.Context(),
			`insert into posts (user_id, body) values ($1, $2) returning id, created_at`,
			post.UserID, post.Body).Scan(&post.ID, &post.CreatedAt)
		if err != nil {
			return err
		}
		args := timeline.FanOutArgs{Author: post.UserID, Post: post.ID}
		return jobs.Enqueue(r.Context(), tx, timeline.FanOut, args)
	})
	if err != nil {
		httpx.ServerError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, post)
}

// Listed is a post in a list: by whom, how many like it, and if the caller does.
type Listed struct {
	Post
	Handle string `json:"handle"`
	Likes  int    `json:"likes"`
	Liked  bool   `json:"liked"`
}

// Page is one page of a user's posts. NextCursor is null on the last page.
type Page struct {
	NextCursor *int64   `json:"next_cursor"`
	Posts      []Listed `json:"posts"`
}

// List returns a page of one user's posts, newest first: 20, or ?limit= up to
// 100, below ?cursor= (a post id).
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
	me := users.ID(r.Context())
	// The author's id first, by itself: then Postgres reads their posts in order,
	// and stops at the limit.
	rows, _ := h.reads.For(r.Context(), me).Query(r.Context(), `
		select p.id, p.user_id, p.body, p.created_at, u.handle, p.likes_count,
			exists (select 1 from likes where post_id = p.id and user_id = $4)
		from posts p
		join users u on u.id = p.user_id
		where p.user_id = (select id from users where handle = $1) and p.id < $2
		order by p.id desc
		limit $3`, r.PathValue("handle"), cursor, limit+1, me)
	posts, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Listed])
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
