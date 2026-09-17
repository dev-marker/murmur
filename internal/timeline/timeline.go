// Package timeline keeps a ready-made home feed for every reader in Redis:
// a sorted set of post ids, written when somebody they follow posts.
package timeline

import (
	"context"
	"math"
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
}

func New(db *pgxpool.Pool, rdb *redis.Client) *Store {
	return &Store{db: db, rdb: rdb}
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
	rows, _ := s.db.Query(ctx, `select follower_id from follows where followee_id = $1`, author)
	followers, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
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
		return s.fromFollows(ctx, user, cursor, n)
	}
	return ids, nil
}

// fromFollows is the slow way to a feed, straight from Postgres: the newest posts
// of every followed account, sorted together.
func (s *Store) fromFollows(ctx context.Context, user, cursor int64, n int) ([]int64, error) {
	rows, _ := s.db.Query(ctx, `
		select p.id
		from follows f
		cross join lateral (
			select id
			from posts
			where user_id = f.followee_id and id < $2
			order by id desc
			limit $3
		) p
		where f.follower_id = $1
		order by p.id desc
		limit $3`, user, cursor, n)
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

// build fills a reader's timeline from Postgres.
func (s *Store) build(ctx context.Context, user int64) error {
	// The marker goes in first. From here on the timeline exists, so new posts reach it.
	zs := []redis.Z{{Score: 0, Member: 0}}
	if err := s.rdb.ZAdd(ctx, key(user), zs...).Err(); err != nil {
		return err
	}
	ids, err := s.fromFollows(ctx, user, math.MaxInt64, Max)
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
