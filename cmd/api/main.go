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
	"github.com/fiston-user/murmur/internal/live"
	"github.com/fiston-user/murmur/internal/notify"
	"github.com/fiston-user/murmur/internal/platform/health"
	"github.com/fiston-user/murmur/internal/platform/metrics"
	"github.com/fiston-user/murmur/internal/platform/postgres"
	"github.com/fiston-user/murmur/internal/platform/redisx"
	"github.com/fiston-user/murmur/internal/platform/replica"
	"github.com/fiston-user/murmur/internal/posts"
	"github.com/fiston-user/murmur/internal/ratelimit"
	"github.com/fiston-user/murmur/internal/timeline"
	"github.com/fiston-user/murmur/internal/users"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	private := cmp.Or(os.Getenv("PRIVATE_ADDR"), ":9081")
	if len(os.Args) > 1 && os.Args[1] == "ready" {
		if err := health.Probe(private); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := run(private); err != nil {
		slog.Error("murmur stopped", "error", err)
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
	addr := cmp.Or(os.Getenv("ADDR"), ":8081")

	pool, err := postgres.Connect(ctx, url, 0)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer pool.Close()
	if err := postgres.Migrate(url); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	hl := health.New(pool.Ping)
	reg := metrics.New()
	metrics.Pool(reg, pool)
	reg.MustRegister(metrics.NewQueue(pool))

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

	// Reads that may be a moment old go to the replica, when there is one.
	var spare *pgxpool.Pool
	if url := os.Getenv("REPLICA_URL"); url != "" {
		if spare, err = postgres.Connect(ctx, url, 0); err != nil {
			return fmt.Errorf("connect to the replica: %w", err)
		}
		defer spare.Close()
	}
	reads := replica.New(pool, spare, rdb)
	go reads.Watch(ctx)

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
	const away = "1 while this limit is counted in this server's memory, because Redis is away."
	metrics.Gauge(reg, "murmur_ratelimit_fallback", away, perIP.Away, "limit", "ip")
	metrics.Gauge(reg, "murmur_ratelimit_fallback", away, perUser.Away, "limit", "user")
	byUser := func(r *http.Request) string { return strconv.FormatInt(users.ID(r.Context()), 10) }

	mux := http.NewServeMux()
	u := users.New(pool, timelines)
	u.Remember = time.Minute
	// Signed in: first who it is, then if they have a token left.
	authed := func(next http.HandlerFunc) http.HandlerFunc {
		return u.Auth(ratelimit.Limit(perUser, byUser, next))
	}
	// A write marks its author first, so that their next reads go to the primary.
	writes := func(next http.HandlerFunc) http.HandlerFunc {
		return authed(func(w http.ResponseWriter, r *http.Request) {
			reads.Wrote(r.Context(), users.ID(r.Context()))
			next(w, r)
		})
	}
	mux.HandleFunc("POST /users", u.Create)
	mux.HandleFunc("POST /users/{handle}/follow", writes(u.Follow))
	mux.HandleFunc("DELETE /users/{handle}/follow", writes(u.Unfollow))
	p := posts.New(pool, reads)
	mux.HandleFunc("POST /posts", writes(p.Create))
	mux.HandleFunc("GET /users/{handle}/posts", u.Maybe(p.List))
	l := likes.New(pool)
	mux.HandleFunc("POST /posts/{id}/like", writes(l.Like))
	mux.HandleFunc("DELETE /posts/{id}/like", writes(l.Unlike))
	n := notify.New(pool, rdb)
	mux.HandleFunc("GET /notifications", authed(n.List))
	mux.HandleFunc("GET /notifications/unread", authed(n.Unread))
	mux.HandleFunc("POST /notifications/read", writes(n.Read))
	f := feed.New(reads, timelines)
	mux.HandleFunc("GET /feed", authed(f.List))
	hub := live.NewHub()
	go hub.Subscribe(ctx, rdb)
	metrics.Gauge(reg, "murmur_live_streams", "Streams that are open on this server.", hub.Open)
	metrics.Counter(reg, "murmur_live_dropped_total", "Streams we let go of.", hub.Dropped)
	lv := &live.Handler{Done: ctx.Done(), Hub: hub, User: users.ID, Missed: n.Since}
	mux.HandleFunc("GET /live", authed(lv.Stream))
	measured := metrics.NewHTTP(reg)

	srv := &http.Server{
		Addr:              addr,
		Handler:           measured.Wrap(ratelimit.Limit(perIP, trusted.ByIP, mux.ServeHTTP)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       time.Minute,
	}
	errc := make(chan error, 2)
	go func() { errc <- srv.ListenAndServe() }()
	// the private port outlives the signal: it says "not ready" while we shut down
	life, end := context.WithCancel(context.Background())
	defer end()
	go func() {
		if err := metrics.Serve(life, private, reg, hl.Routes); err != nil {
			errc <- err
		}
	}()
	slog.Info("listening", "addr", addr, "private", private)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		stop()
	}
	slog.Info("shutting down")
	hl.Leave()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// limiter makes a rate limiter from two settings, or from their defaults. For "ip",
// IP_BURST is how many requests may come at once, and IP_PER_MINUTE is the steady rate.
// The buckets live in Redis, and in this server's memory while Redis is away.
func limiter(rdb *redis.Client, name string, burst, perMinute int) (*ratelimit.Fallback, error) {
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
