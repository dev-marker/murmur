package replica

import (
	"context"
	"testing"

	"github.com/fiston-user/murmur/internal/platform/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Two pools on the one test database stand in for the primary and the replica.
// What matters is which of the two a read gets.
func TestFor(t *testing.T) {
	ctx := context.Background()
	primary, spare, rdb := testdb.New(t), testdb.New(t), testdb.Redis(t)
	r := New(primary, spare, rdb)
	r.usable.Store(true)
	for _, step := range []struct {
		name string
		do   func()
		user int64
		want *pgxpool.Pool
	}{
		{"nobody reads the replica", func() {}, 0, spare},
		{"so does a user who wrote nothing", func() {}, 7, spare},
		{"a user who just wrote reads the primary", func() { r.Wrote(ctx, 7) }, 7, primary},
		{"everybody else still reads the replica", func() {}, 8, spare},
		{"a replica that is behind is read by nobody", func() { r.usable.Store(false) }, 8, primary},
	} {
		step.do()
		if got := r.For(ctx, step.user); got != step.want {
			t.Errorf("%s: got the wrong pool", step.name)
		}
	}
}

// A database that is not a replica is never taken for one.
func TestCheck(t *testing.T) {
	primary := testdb.New(t)
	r := New(primary, primary, nil)
	r.usable.Store(true)
	r.check(context.Background())
	if r.usable.Load() {
		t.Error("the primary was taken for a replica")
	}
}
