package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/fiston-user/murmur/internal/jobs"
	"github.com/fiston-user/murmur/internal/platform/postgres"
	"github.com/fiston-user/murmur/internal/platform/redisx"
	"github.com/fiston-user/murmur/internal/timeline"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return errors.New("DATABASE_URL is not set")
	}
	pool, err := postgres.Connect(ctx, url)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer pool.Close()

	rdb, err := redisx.Connect(ctx, os.Getenv("REDIS_URL"))
	if err != nil {
		return fmt.Errorf("connect to redis: %w", err)
	}
	defer rdb.Close()
	big, err := strconv.Atoi(cmp.Or(os.Getenv("BIG_ACCOUNT"), "1000"))
	if err != nil {
		return fmt.Errorf("BIG_ACCOUNT: %w", err)
	}
	timelines := timeline.New(pool, rdb, big)

	w := jobs.NewWorker(pool)
	w.Handle(timeline.FanOut, timelines.RunFanOut)
	slog.Info("working")
	w.Run(ctx, time.Second)
	slog.Info("shutting down")
	return nil
}
