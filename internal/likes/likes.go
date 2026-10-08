// Package likes is liking a post, and counting the likes.
package likes

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/fiston-user/murmur/internal/jobs"
	"github.com/fiston-user/murmur/internal/notify"
	"github.com/fiston-user/murmur/internal/platform/httpx"
	"github.com/fiston-user/murmur/internal/users"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// postID reads the post's id from the path, or answers 404.
func postID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "no such post")
		return 0, false
	}
	return id, true
}

// Like makes the caller like the post in the path. Doing it twice is fine.
func (h *Handler) Like(w http.ResponseWriter, r *http.Request) {
	post, ok := postID(w, r)
	if !ok {
		return
	}
	user := users.ID(r.Context())
	err := pgx.BeginFunc(r.Context(), h.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(r.Context(),
			`insert into likes (post_id, user_id) values ($1, $2) on conflict do nothing`, post, user)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		args := notify.LikeArgs{Liker: user, Post: post}
		if err := jobs.Enqueue(r.Context(), tx, notify.Like, args); err != nil {
			return err
		}
		_, err = tx.Exec(r.Context(),
			`insert into like_deltas (post_id, delta) values ($1, 1)`, post)
		return err
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" { // foreign_key_violation
		httpx.Error(w, http.StatusNotFound, "no such post")
		return
	}
	if err != nil {
		httpx.ServerError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Unlike takes the caller's like back. Without a like, nothing happens.
func (h *Handler) Unlike(w http.ResponseWriter, r *http.Request) {
	post, ok := postID(w, r)
	if !ok {
		return
	}
	user := users.ID(r.Context())
	err := pgx.BeginFunc(r.Context(), h.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(r.Context(),
			`delete from likes where post_id = $1 and user_id = $2`, post, user)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		_, err = tx.Exec(r.Context(),
			`insert into like_deltas (post_id, delta) values ($1, -1)`, post)
		return err
	})
	if err != nil {
		httpx.ServerError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Fold adds every delta that waits to its post's count, and deletes it, in one statement.
// So a delta is counted once, or it's still waiting. It returns how many posts changed.
func Fold(ctx context.Context, db *pgxpool.Pool) (int64, error) {
	tag, err := db.Exec(ctx, `
		with gone as (
			delete from like_deltas returning post_id, delta
		), sums as (
			select post_id, sum(delta) as n from gone group by post_id
		)
		update posts p set likes_count = p.likes_count + s.n
		from sums s
		where p.id = s.post_id`)
	return tag.RowsAffected(), err
}

// FoldEvery folds again and again, until ctx is cancelled.
func FoldEvery(ctx context.Context, db *pgxpool.Pool, every time.Duration) {
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if _, err := Fold(ctx, db); err != nil {
				slog.Error("likes were not folded", "error", err)
			}
		}
	}
}
