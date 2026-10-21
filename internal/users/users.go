// Package users is sign-up and bearer tokens.
package users

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fiston-user/murmur/internal/platform/httpx"
	"github.com/fiston-user/murmur/internal/timeline"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var validHandle = regexp.MustCompile(`^[a-z0-9_]{3,20}$`)

type Handler struct {
	db *pgxpool.Pool
	tl *timeline.Store

	// Remember is how long a token that we looked up is believed.
	// Zero means every request asks Postgres, like before.
	Remember time.Duration
	now      func() time.Time
	known    sync.Map // token hash -> session
}

// A session is what we remember about a token.
type session struct {
	user  int64
	until time.Time
}

func New(db *pgxpool.Pool, tl *timeline.Store) *Handler {
	return &Handler{db: db, tl: tl, now: time.Now}
}

// Create signs a user up and answers with a token, shown once.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Handle string `json:"handle"`
	}
	if err := httpx.Decode(w, r, &in); err != nil || !validHandle.MatchString(in.Handle) {
		httpx.Error(w, http.StatusBadRequest, "handle must be 3 to 20 of a-z, 0-9 and _")
		return
	}
	b := make([]byte, 32)
	rand.Read(b)
	token := base64.RawURLEncoding.EncodeToString(b)
	hash := sha256.Sum256([]byte(token))

	var id int64
	err := h.db.QueryRow(r.Context(),
		`insert into users (handle, token_hash) values ($1, $2) returning id`,
		in.Handle, hash[:]).Scan(&id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
		httpx.Error(w, http.StatusConflict, "that handle is taken")
		return
	}
	if err != nil {
		httpx.ServerError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"id": id, "handle": in.Handle, "token": token})
}

type ctxKey struct{}

// Auth lets a request through only with a valid bearer token.
func (h *Handler) Auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			httpx.Error(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		hash := sha256.Sum256([]byte(token))
		if s, ok := h.known.Load(hash); ok && h.now().Before(s.(session).until) {
			next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, s.(session).user)))
			return
		}

		var id int64
		err := h.db.QueryRow(r.Context(), `select id from users where token_hash = $1`, hash[:]).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, http.StatusUnauthorized, "invalid token")
			return
		}
		if err != nil {
			httpx.ServerError(w, r, err)
			return
		}
		if h.Remember > 0 {
			h.known.Store(hash, session{user: id, until: h.now().Add(h.Remember)})
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
	}
}

// ID returns the id of the user Auth let through.
func ID(ctx context.Context) int64 {
	id, _ := ctx.Value(ctxKey{}).(int64)
	return id
}
