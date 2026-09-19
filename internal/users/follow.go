package users

import (
	"errors"
	"net/http"

	"github.com/fiston-user/murmur/internal/platform/httpx"
	"github.com/jackc/pgx/v5"
)

// target finds the user named in the path, or answers 404.
func (h *Handler) target(w http.ResponseWriter, r *http.Request) (int64, bool) {
	var id int64
	err := h.db.QueryRow(r.Context(),
		`select id from users where handle = $1`, r.PathValue("handle")).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, http.StatusNotFound, "no such user")
		return 0, false
	}
	if err != nil {
		httpx.ServerError(w, r, err)
		return 0, false
	}
	return id, true
}

// Follow makes the caller follow the user in the path. Doing it twice is fine.
func (h *Handler) Follow(w http.ResponseWriter, r *http.Request) {
	followee, ok := h.target(w, r)
	if !ok {
		return
	}
	if followee == ID(r.Context()) {
		httpx.Error(w, http.StatusBadRequest, "you can't follow yourself")
		return
	}
	_, err := h.db.Exec(r.Context(), `
		with added as (
			insert into follows (follower_id, followee_id) values ($1, $2)
			on conflict do nothing
			returning followee_id
		)
		update users set followers_count = followers_count + 1
		where id in (select followee_id from added)`,
		ID(r.Context()), followee)
	if err != nil {
		httpx.ServerError(w, r, err)
		return
	}
	// Who they follow just changed, so their timeline is out of date.
	if err := h.tl.Drop(r.Context(), ID(r.Context())); err != nil {
		httpx.ServerError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Unfollow undoes a follow. Unfollowing someone you don't follow is fine too.
func (h *Handler) Unfollow(w http.ResponseWriter, r *http.Request) {
	followee, ok := h.target(w, r)
	if !ok {
		return
	}
	_, err := h.db.Exec(r.Context(), `
		with removed as (
			delete from follows where follower_id = $1 and followee_id = $2
			returning followee_id
		)
		update users set followers_count = followers_count - 1
		where id in (select followee_id from removed)`,
		ID(r.Context()), followee)
	if err != nil {
		httpx.ServerError(w, r, err)
		return
	}
	// Who they follow just changed, so their timeline is out of date.
	if err := h.tl.Drop(r.Context(), ID(r.Context())); err != nil {
		httpx.ServerError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
