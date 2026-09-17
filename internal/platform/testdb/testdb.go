// Package testdb gives integration tests a real Postgres database, and a real Redis.
package testdb

import (
	"context"
	"errors"
	"testing"

	"github.com/fiston-user/murmur/internal/platform/postgres"
	"github.com/fiston-user/murmur/internal/platform/redisx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// The Compose Postgres (make up). Tests get a database of their own, never the one the API uses.
const (
	adminURL = "postgres://murmur:murmur@localhost:5432/murmur?sslmode=disable&connect_timeout=5"
	testURL  = "postgres://murmur:murmur@localhost:5432/murmur_test?sslmode=disable&connect_timeout=5"
)

// New returns a pool on murmur_test, migrated, with every table empty.
func New(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect to postgres (did you run make up?): %v", err)
	}
	defer admin.Close(ctx)
	_, err = admin.Exec(ctx, `create database murmur_test`)
	var pgErr *pgconn.PgError
	if err != nil && !(errors.As(err, &pgErr) && pgErr.Code == "42P04") { // duplicate_database
		t.Fatalf("create murmur_test: %v", err)
	}

	if err := postgres.Migrate(testURL); err != nil {
		t.Fatalf("migrate murmur_test: %v", err)
	}
	pool, err := postgres.Connect(ctx, testURL)
	if err != nil {
		t.Fatalf("connect to murmur_test: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `truncate users, posts, follows restart identity`); err != nil {
		t.Fatalf("empty the tables: %v", err)
	}
	return pool
}

// Redis returns a client on database 1 of the Compose Redis, emptied. The API uses database 0.
func Redis(t *testing.T) *redis.Client {
	t.Helper()
	ctx := context.Background()
	rdb, err := redisx.Connect(ctx, "redis://localhost:6379/1")
	if err != nil {
		t.Fatalf("connect to redis (did you run make up?): %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	if err := rdb.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("empty redis database 1: %v", err)
	}
	return rdb
}
