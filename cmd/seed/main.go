// Command seed fills the database with a made-up social graph, for load tests.
// The same flags always build the same graph.
package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"time"

	"github.com/fiston-user/murmur/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

func main() {
	users := flag.Int("users", 100_000, "how many users to create")
	seed := flag.Uint64("seed", 1, "the same seed always builds the same graph")
	flag.Parse()
	if err := run(context.Background(), *users, *seed); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

// load copies rows into one table, and prints how many.
func load(ctx context.Context, conn *pgx.Conn, table string, cols []string, src pgx.CopyFromSource) error {
	start := time.Now()
	n, err := conn.CopyFrom(ctx, pgx.Identifier{table}, cols, src)
	if err != nil {
		return fmt.Errorf("copy %s: %w", table, err)
	}
	fmt.Printf("%-8s %9d rows  %5.1fs\n", table, n, time.Since(start).Seconds())
	return nil
}

func run(ctx context.Context, users int, seed uint64) error {
	if users < 1000 {
		return errors.New("--users must be at least 1000")
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return errors.New("DATABASE_URL is not set")
	}
	if err := postgres.Migrate(url); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer conn.Close(ctx)

	// Start from nothing, so the first user is always id 1.
	if _, err := conn.Exec(ctx, `
		truncate users, posts, follows, jobs, likes, like_deltas, notifications
		restart identity`); err != nil {
		return err
	}
	// We only write ids we just made: skip the foreign key checks.
	if _, err := conn.Exec(ctx, `set session_replication_role = replica`); err != nil {
		return err
	}

	r := rand.New(rand.NewPCG(seed, 0))
	big := rand.NewZipf(r, 1.2, 5, uint64(users-1))
	// pick chooses an account: one time in five a big name, otherwise anybody.
	pick := func() int64 {
		if r.IntN(5) == 0 {
			return int64(big.Uint64()) + 1
		}
		return r.Int64N(int64(users)) + 1
	}

	err = load(ctx, conn, "users", []string{"handle", "token_hash"},
		pgx.CopyFromSlice(users, func(i int) ([]any, error) {
			hash := sha256.Sum256(fmt.Appendf(nil, "seed-token-%d", i+1))
			return []any{fmt.Sprintf("user%d", i+1), hash[:]}, nil
		}))
	if err != nil {
		return err
	}

	follower, left := int64(0), 0
	seen := map[int64]bool{}
	err = load(ctx, conn, "follows", []string{"follower_id", "followee_id"},
		pgx.CopyFromFunc(func() ([]any, error) {
			if left == 0 {
				if follower++; follower > int64(users) {
					return nil, nil
				}
				left = 10 + int(190*math.Pow(r.Float64(), 4))
				clear(seen)
			}
			for {
				if id := pick(); id != follower && !seen[id] {
					seen[id] = true
					left--
					return []any{follower, id}, nil
				}
			}
		}))
	if err != nil {
		return err
	}

	// Every user's follower count, from the follows we just loaded.
	_, err = conn.Exec(ctx, `
		update users u
		set followers_count = c.n
		from (select followee_id, count(*) as n from follows group by followee_id) c
		where u.id = c.followee_id`)
	if err != nil {
		return err
	}

	first := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	err = load(ctx, conn, "posts", []string{"user_id", "body", "created_at"},
		pgx.CopyFromSlice(20*users, func(i int) ([]any, error) {
			author := pick()
			body := fmt.Sprintf("post %d, by user%d", i+1, author)
			return []any{author, body, first.Add(time.Duration(i) * time.Second)}, nil
		}))
	if err != nil {
		return err
	}

	// Fresh statistics for the planner, before anyone measures.
	_, err = conn.Exec(ctx, `vacuum analyze`)
	return err
}
