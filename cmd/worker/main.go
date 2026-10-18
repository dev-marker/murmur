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
	"github.com/fiston-user/murmur/internal/likes"
	"github.com/fiston-user/murmur/internal/notify"
	"github.com/fiston-user/murmur/internal/platform/health"
	"github.com/fiston-user/murmur/internal/platform/metrics"
	"github.com/fiston-user/murmur/internal/platform/postgres"
	"github.com/fiston-user/murmur/internal/platform/redisx"
	"github.com/fiston-user/murmur/internal/timeline"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	private := cmp.Or(os.Getenv("PRIVATE_ADDR"), ":9101")
	if len(os.Args) > 1 && os.Args[1] == "ready" {
		if err := health.Probe(private); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := run(private); err != nil {
		slog.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func run(private string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return errors.New("DATABASE_URL is not set")
	}
	workers, err := strconv.Atoi(cmp.Or(os.Getenv("WORKERS"), "4"))
	if err != nil || workers < 1 {
		return fmt.Errorf("WORKERS: %q is not a number of workers", os.Getenv("WORKERS"))
	}
	pool, err := postgres.Connect(ctx, url, workers+2)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer pool.Close()
	reg := metrics.New()
	metrics.Pool(reg, pool)

	rdb, err := redisx.Connect(ctx, os.Getenv("REDIS_URL"))
	if err != nil {
		return fmt.Errorf("connect to redis: %w", err)
	}
	defer rdb.Close()
	metrics.Redis(reg, rdb)
	big, err := strconv.Atoi(cmp.Or(os.Getenv("BIG_ACCOUNT"), "1000"))
	if err != nil {
		return fmt.Errorf("BIG_ACCOUNT: %w", err)
	}
	timelines := timeline.New(pool, rdb, big)
	timeout, err := time.ParseDuration(cmp.Or(os.Getenv("JOB_TIMEOUT"), "1m"))
	if err != nil {
		return fmt.Errorf("JOB_TIMEOUT: %w", err)
	}
	grace, err := time.ParseDuration(cmp.Or(os.Getenv("JOB_GRACE"), "20s"))
	if err != nil {
		return fmt.Errorf("JOB_GRACE: %w", err)
	}

	w := jobs.NewWorker(pool)
	w.Ended = metrics.Jobs(reg)
	w.Handle(timeline.FanOut, timelines.RunFanOut)
	w.Handle(notify.Like, notify.New(pool, rdb).RunLike)
	go likes.FoldEvery(ctx, pool, time.Second)
	hl := health.New(pool.Ping)
	go func() {
		if err := metrics.Serve(ctx, private, reg, hl.Routes); err != nil {
			slog.Error("the private port is closed", "error", err)
			stop()
		}
	}()
	slog.Info("working", "workers", workers, "connections", pool.Config().MaxConns, "private", private)
	w.Run(ctx, workers, time.Second, timeout, grace)
	slog.Info("shutting down")
	return nil
}
