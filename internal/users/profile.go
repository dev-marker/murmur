package users

import (
	"errors"
	"net/http"
	"time"

	"github.com/fiston-user/murmur/internal/platform/httpx"
	"github.com/fiston-user/murmur/internal/platform/replica"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Profiles answers what a client asks about accounts. Most of it may be
// a moment old, so it goes wherever reads.For sends it.
type Profiles struct {
	db    *pgxpool.Pool
	reads *replica.Reads
}

func NewProfiles(db *pgxpool.Pool, reads *replica.Reads) *Profiles {
	return &Profiles{db: db, reads: reads}
}

// Me says who the token belongs to. It asks the primary, like Auth does.
func (p *Profiles) Me(w http.ResponseWriter, r *http.Request) {
	id := ID(r.Context())
	var handle string
	err := p.db.QueryRow(r.Context(), `select handle from users where id = $1`, id).Scan(&handle)
	if err != nil {
		httpx.ServerError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"id": id, "handle": handle})
}

// Profile is one account, the way its page shows it.
type Profile struct {
	Handle       string    `json:"handle"`
	Joined       time.Time `json:"joined"`
	Followers    int       `json:"followers"`
	Following    int       `json:"following"`
	Posts        int       `json:"posts"`
	FollowedByMe bool      `json:"followed_by_me"`
}

// Show returns the profile of the user in the path, to anybody.
func (p *Profiles) Show(w http.ResponseWriter, r *http.Request) {
	me := ID(r.Context())
	rows, _ := p.reads.For(r.Context(), me).Query(r.Context(), `
		select u.handle, u.created_at, u.followers_count,
			(select count(*) from follows where follower_id = u.id),
			(select count(*) from posts where user_id = u.id),
			exists (select 1 from follows where follower_id = $2 and followee_id = u.id)
		from users u
		where u.handle = $1`, r.PathValue("handle"), me)
	profile, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[Profile])
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, http.StatusNotFound, "no such user")
		return
	}
	if err != nil {
		httpx.ServerError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, profile)
}

// Suggestion is an account worth following.
type Suggestion struct {
	Handle    string `json:"handle"`
	Followers int    `json:"followers"`
}

// Suggest returns three of the most followed accounts that the caller
// doesn't follow yet, picked from the top fifty.
func (p *Profiles) Suggest(w http.ResponseWriter, r *http.Request) {
	me := ID(r.Context())
	rows, _ := p.reads.For(r.Context(), me).Query(r.Context(), `
		select handle, followers_count
		from (select id, handle, followers_count from users
			order by followers_count desc limit 50) top
		where id <> $1
		  and not exists (
			select 1 from follows where follower_id = $1 and followee_id = top.id)
		order by followers_count desc
		limit 3`, me)
	list, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Suggestion])
	if err != nil {
		httpx.ServerError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"users": list})
}
