// Package postgres opens the pool and runs the migrations.
package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/fiston-user/murmur/db"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a pool of at least conns connections, and checks that the database answers.
func Connect(ctx context.Context, url string, conns int) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = max(cfg.MaxConns, int32(conns))
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// Migrate applies the migrations that haven't run yet.
func Migrate(url string) error {
	src, err := iofs.New(db.Migrations, "migrations")
	if err != nil {
		return err
	}
	target := strings.Replace(url, "postgres://", "pgx5://", 1)
	m, err := migrate.NewWithSourceInstance("iofs", src, target)
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}
