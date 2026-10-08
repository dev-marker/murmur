package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fiston-user/murmur/internal/feed"
	"github.com/fiston-user/murmur/internal/likes"
	"github.com/fiston-user/murmur/internal/notify"
	"github.com/fiston-user/murmur/internal/platform/postgres"
	"github.com/fiston-user/murmur/internal/platform/redisx"
	"github.com/fiston-user/murmur/internal/posts"
	"github.com/fiston-user/murmur/internal/ratelimit"
	"github.com/fiston-user/murmur/internal/timeline"
	"github.com/fiston-user/murmur/internal/users"
	"github.com/redis/go-redis/v9"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("murmur stopped", "error", err)
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
	addr := cmp.Or(os.Getenv("ADDR"), ":8081")

	pool, err := postgres.Connect(ctx, url, 0)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer pool.Close()
	if err := postgres.Migrate(url); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

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

	trusted, err := proxies()
	if err != nil {
		return err
	}

	perIP, err := limiter(rdb, "ip", 100, 3000)
	if err != nil {
		return err
	}
	perUser, err := limiter(rdb, "user", 20, 60)
	if err != nil {
		return err
	}
	byUser := func(r *http.Request) string { return strconv.FormatInt(users.ID(r.Context()), 10) }

	mux := http.NewServeMux()
	u := users.New(pool, timelines)
	// Signed in: first who it is, then if they have a token left.
	authed := func(next http.HandlerFunc) http.HandlerFunc {
		return u.Auth(ratelimit.Limit(perUser, byUser, next))
	}
	mux.HandleFunc("POST /users", u.Create)
	mux.HandleFunc("POST /users/{handle}/follow", authed(u.Follow))
	mux.HandleFunc("DELETE /users/{handle}/follow", authed(u.Unfollow))
	p := posts.New(pool)
	mux.HandleFunc("POST /posts", authed(p.Create))
	mux.HandleFunc("GET /users/{handle}/posts", p.List)
	l := likes.New(pool)
	mux.HandleFunc("POST /posts/{id}/like", authed(l.Like))
	mux.HandleFunc("DELETE /posts/{id}/like", authed(l.Unlike))
	n := notify.New(pool)
	mux.HandleFunc("GET /notifications", authed(n.List))
	mux.HandleFunc("GET /notifications/unread", authed(n.Unread))
	mux.HandleFunc("POST /notifications/read", authed(n.Read))
	f := feed.New(pool, timelines)
	mux.HandleFunc("GET /feed", authed(f.List))

	srv := &http.Server{
		Addr:              addr,
		Handler:           ratelimit.Limit(perIP, trusted.ByIP, mux.ServeHTTP),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       time.Minute,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	slog.Info("listening", "addr", addr)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		stop()
	}
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// limiter makes a rate limiter from two settings, or from their defaults. For "ip",
// IP_BURST is how many requests may come at once, and IP_PER_MINUTE is the steady rate.
// The buckets live in Redis, and in this server's memory while Redis is away.
func limiter(rdb *redis.Client, name string, burst, perMinute int) (ratelimit.Allower, error) {
	var err error
	if burst, err = setting(strings.ToUpper(name)+"_BURST", burst); err != nil {
		return nil, err
	}
	if perMinute, err = setting(strings.ToUpper(name)+"_PER_MINUTE", perMinute); err != nil {
		return nil, err
	}
	shared := ratelimit.NewShared(rdb, name, burst, perMinute)
	return ratelimit.NewFallback(shared, ratelimit.New(name, burst, perMinute)), nil
}

// setting reads a positive whole number from the environment, or takes the default.
func setting(env string, def int) (int, error) {
	n, err := strconv.Atoi(cmp.Or(os.Getenv(env), strconv.Itoa(def)))
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%s: %q is not a positive number", env, os.Getenv(env))
	}
	return n, nil
}

// proxies reads TRUSTED_PROXIES: the addresses our load balancers connect from, as
// prefixes like 10.0.0.0/8 with commas between them. Nobody else is believed about
// X-Forwarded-For.
func proxies() (ratelimit.Proxies, error) {
	var ps ratelimit.Proxies
	for s := range strings.SplitSeq(os.Getenv("TRUSTED_PROXIES"), ",") {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXIES: %w", err)
		}
		ps = append(ps, p)
	}
	return ps, nil
}
