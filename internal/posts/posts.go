// Package posts writes and lists posts.
package posts

import (
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/fiston-user/murmur/internal/jobs"
	"github.com/fiston-user/murmur/internal/platform/httpx"
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
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
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

// List returns one user's posts, newest first, 20 at most.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	rows, _ := h.db.Query(r.Context(), `
		select id, user_id, body, created_at
		from posts
		where user_id = (select id from users where handle = $1)
		order by id desc
		limit 20`, r.PathValue("handle"))
	posts, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Post])
	if err != nil {
		httpx.ServerError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"posts": posts})
}
