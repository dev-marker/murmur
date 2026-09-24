// Package timeline keeps a ready-made home feed for every reader in Redis:
// a sorted set of post ids, written when somebody they follow posts.
package timeline

import (
	"cmp"
	"context"
	"encoding/json"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Max is how many posts one timeline keeps.
const Max = 800

// ttl is how long a timeline lives. After that, the next read builds a fresh one.
const ttl = 7 * 24 * time.Hour

type Store struct {
	db  *pgxpool.Pool
	rdb *redis.Client
	big int // followers that make an account big: its posts are pulled, not pushed
}

func New(db *pgxpool.Pool, rdb *redis.Client, big int) *Store {
	return &Store{db: db, rdb: rdb, big: big}
}

// key is where a reader's timeline lives in Redis.
func key(user int64) string {
	return "timeline:" + strconv.FormatInt(user, 10)
}

// add puts a post into a timeline that exists. A reader without one gets none.
var add = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then
	return 0
end
redis.call('ZADD', KEYS[1], ARGV[1], ARGV[1])
return redis.call('ZREMRANGEBYRANK', KEYS[1], 1, -ARGV[2] - 1)`)

// Push adds a post to the timelines of the people who follow its author.
func (s *Store) Push(ctx context.Context, author, post int64) error {
	// A big account gets no rows here: its posts are pulled when a feed is read.
	rows, _ := s.db.Query(ctx, `
		select follower_id
		from follows
		where followee_id = $1
		  and (select followers_count from users where id = $1) < $2`, author, s.big)
	followers, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil || len(followers) == 0 {
		return err
	}
	// Redis gets the script once. After that, the pipeline calls it by its hash.
	if err := add.Load(ctx, s.rdb).Err(); err != nil {
		return err
	}
	pipe := s.rdb.Pipeline()
	for _, id := range followers {
		add.EvalSha(ctx, pipe, []string{key(id)}, post, Max)
	}
	_, err = pipe.Exec(ctx)
	return err
}

// FanOut is the kind of job that pushes one post to its author's followers.
const FanOut = "fanout"

// FanOutArgs is that job's payload.
type FanOutArgs struct {
	Author int64 `json:"author"`
	Post   int64 `json:"post"`
}

// RunFanOut handles a FanOut job: it pushes the post in the payload.
func (s *Store) RunFanOut(ctx context.Context, payload []byte) error {
	var a FanOutArgs
	if err := json.Unmarshal(payload, &a); err != nil {
		return err
	}
	return s.Push(ctx, a.Author, a.Post)
}

// Page returns up to n post ids below cursor from a reader's feed, newest first.
func (s *Store) Page(ctx context.Context, user, cursor int64, n int) ([]int64, error) {
	below := redis.ZRangeArgs{
		Key: key(user), Start: "(" + strconv.FormatInt(cursor, 10), Stop: 0,
		ByScore: true, Rev: true, Count: int64(n),
	}
	var ids []int64
	err := s.rdb.ZRangeArgs(ctx, below).ScanSlice(&ids)
	if err == nil && len(ids) == 0 { // not even the marker: there's no timeline
		if err = s.build(ctx, user); err == nil {
			err = s.rdb.ZRangeArgs(ctx, below).ScanSlice(&ids)
		}
	}
	if err != nil {
		return nil, err
	}
	if len(ids) < n || ids[n-1] == 0 { // the timeline ends here: Postgres has the rest
		return s.fromFollows(ctx, user, cursor, n, 0)
	}
	pulled, err := s.fromFollows(ctx, user, cursor, n, s.big)
	if err != nil {
		return nil, err
	}
	return merge(ids, pulled, n), nil
}

// merge joins two lists of post ids into one, newest first, n at most.
// A post that is in both lists comes out once.
func merge(a, b []int64, n int) []int64 {
	ids := slices.Concat(a, b)
	slices.SortFunc(ids, func(x, y int64) int { return cmp.Compare(y, x) })
	ids = slices.Compact(ids)
	return ids[:min(n, len(ids))]
}

// fromFollows is the slow way to a feed, straight from Postgres: the newest posts
// of every followed account with at least atLeast followers, sorted together.
func (s *Store) fromFollows(ctx context.Context, user, cursor int64, n, atLeast int) ([]int64, error) {
	rows, _ := s.db.Query(ctx, `
		select p.id
		from follows f
		join users u on u.id = f.followee_id and u.followers_count >= $4
		cross join lateral (
			select id
			from posts
			where user_id = f.followee_id and id < $2
			order by id desc
			limit $3
		) p
		where f.follower_id = $1
		order by p.id desc
		limit $3`, user, cursor, n, atLeast)
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

// build fills a reader's timeline from Postgres.
func (s *Store) build(ctx context.Context, user int64) error {
	// The marker goes in first. From here on the timeline exists, so new posts reach it.
	zs := []redis.Z{{Score: 0, Member: 0}}
	if err := s.rdb.ZAdd(ctx, key(user), zs...).Err(); err != nil {
		return err
	}
	ids, err := s.fromFollows(ctx, user, math.MaxInt64, Max, 0)
	if err != nil {
		return err
	}
	for _, id := range ids {
		zs = append(zs, redis.Z{Score: float64(id), Member: id})
	}
	pipe := s.rdb.Pipeline()
	pipe.ZAdd(ctx, key(user), zs...)
	pipe.Expire(ctx, key(user), ttl)
	_, err = pipe.Exec(ctx)
	return err
}

// Drop forgets a reader's timeline. Their next read builds it again.
func (s *Store) Drop(ctx context.Context, user int64) error {
	return s.rdb.Del(ctx, key(user)).Err()
}
