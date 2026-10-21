package users

import (
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fiston-user/murmur/internal/platform/testdb"
)

// A token that was looked up is believed for a while, and then Postgres is asked again.
// The price is in the middle step: a user who is gone can still get in, until that while is over.
func TestAuthRemembers(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	hash := sha256.Sum256([]byte("secret"))
	_, err := db.Exec(ctx, `insert into users (handle, token_hash) values ('alice', $1)`, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	h := New(db, nil)
	h.Remember = time.Minute
	clock := time.Now()
	h.now = func() time.Time { return clock }
	inside := h.Auth(func(w http.ResponseWriter, r *http.Request) {})
	ask := func() int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/feed", nil)
		req.Header.Set("Authorization", "Bearer secret")
		inside(rec, req)
		return rec.Code
	}
	for _, step := range []struct {
		name string
		do   func()
		want int
	}{
		{"the first request asks postgres", func() {}, 200},
		{"the user is deleted, and still remembered", func() { db.Exec(ctx, `delete from users`) }, 200},
		{"a minute later, postgres is asked again", func() { clock = clock.Add(61 * time.Second) }, 401},
	} {
		step.do()
		if got := ask(); got != step.want {
			t.Errorf("%s: got %d, want %d", step.name, got, step.want)
		}
	}
}
