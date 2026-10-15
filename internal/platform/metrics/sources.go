package metrics

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

// A number is anything a gauge or a counter can be read from.
type number interface {
	~int | ~int32 | ~int64 | ~uint32 | ~float64
}

// Gauge registers a number that goes up and down. Nobody keeps it up to date:
// read is called when Prometheus asks. labels are pairs: a name, then its value.
func Gauge[N number](reg prometheus.Registerer, name, help string, read func() N, labels ...string) {
	opts := prometheus.GaugeOpts{Name: name, Help: help, ConstLabels: pairs(labels)}
	reg.MustRegister(prometheus.NewGaugeFunc(opts, func() float64 { return float64(read()) }))
}

// Counter registers a number that only ever goes up, read the same way.
func Counter[N number](reg prometheus.Registerer, name, help string, read func() N, labels ...string) {
	opts := prometheus.CounterOpts{Name: name, Help: help, ConstLabels: pairs(labels)}
	reg.MustRegister(prometheus.NewCounterFunc(opts, func() float64 { return float64(read()) }))
}

func pairs(labels []string) prometheus.Labels {
	set := prometheus.Labels{}
	for i := 0; i+1 < len(labels); i += 2 {
		set[labels[i]] = labels[i+1]
	}
	return set
}

// Pool registers what pgx's connection pool knows about itself.
func Pool(reg prometheus.Registerer, db *pgxpool.Pool) {
	Gauge(reg, "murmur_db_pool_in_use_connections", "Connections that a query holds right now.",
		func() int32 { return db.Stat().AcquiredConns() })
	Gauge(reg, "murmur_db_pool_idle_connections", "Connections that are open, with nothing to do.",
		func() int32 { return db.Stat().IdleConns() })
	Gauge(reg, "murmur_db_pool_max_connections", "The most connections this pool opens.",
		func() int32 { return db.Stat().MaxConns() })
	Counter(reg, "murmur_db_pool_waits_total", "Queries that found no free connection, and waited.",
		func() int64 { return db.Stat().EmptyAcquireCount() })
	Counter(reg, "murmur_db_pool_wait_seconds_total", "How long those queries waited, added up.",
		func() float64 { return db.Stat().EmptyAcquireWaitTime().Seconds() })
}

// Redis registers how long every command to Redis takes and how many fail,
// and what go-redis's connection pool knows about itself.
func Redis(reg prometheus.Registerer, rdb *redis.Client) {
	h := redisHook{
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "murmur_redis_call_seconds",
			Help:    "How long a call to Redis took, by command. A pipeline is one call.",
			Buckets: []float64{.0001, .00025, .0005, .001, .0025, .005, .01, .025, .05, .1, .25, 1, 5},
		}, []string{"command"}),
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "murmur_redis_errors_total",
			Help: "Commands to Redis that failed, by command.",
		}, []string{"command"}),
	}
	reg.MustRegister(h.duration, h.errors)
	rdb.AddHook(h)
	Gauge(reg, "murmur_redis_pool_connections", "Connections to Redis that are open.",
		func() uint32 { return rdb.PoolStats().TotalConns })
	Gauge(reg, "murmur_redis_pool_idle_connections", "Connections to Redis with nothing to do.",
		func() uint32 { return rdb.PoolStats().IdleConns })
	Counter(reg, "murmur_redis_pool_timeouts_total", "Commands that gave up waiting for a connection.",
		func() uint32 { return rdb.PoolStats().Timeouts })
}

// A redisHook sits around every command that go-redis sends.
type redisHook struct {
	duration *prometheus.HistogramVec
	errors   *prometheus.CounterVec
}

func (h redisHook) observe(command string, start time.Time, err error) {
	h.duration.WithLabelValues(command).Observe(time.Since(start).Seconds())
	// Two answers are errors to go-redis, and not to us: "no such key", and
	// "I don't know that script yet", which go-redis follows up with the script itself.
	if err != nil && !errors.Is(err, redis.Nil) && !redis.HasErrorPrefix(err, "NOSCRIPT") {
		h.errors.WithLabelValues(command).Inc()
	}
}

func (h redisHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return next(ctx, network, addr)
	}
}

func (h redisHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		start := time.Now()
		err := next(ctx, cmd)
		h.observe(cmd.Name(), start, err)
		return err
	}
}

func (h redisHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		start := time.Now()
		err := next(ctx, cmds)
		h.observe("pipeline", start, err)
		return err
	}
}

// Jobs returns what a worker calls every time a job ends: how long it waited
// in the queue, how long it ran, and the error if it failed.
func Jobs(reg prometheus.Registerer) func(kind string, waited, ran time.Duration, err error) {
	ended := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "murmur_jobs_total",
		Help: "Jobs that ended, by kind and result.",
	}, []string{"kind", "result"})
	wait := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "murmur_job_wait_seconds",
		Help:    "How long a job sat in the queue before a worker started it.",
		Buckets: []float64{.01, .05, .25, 1, 5, 15, 60, 300},
	})
	run := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "murmur_job_run_seconds",
		Help:    "How long a job's handler took, by kind.",
		Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, 1, 5},
	}, []string{"kind"})
	reg.MustRegister(ended, wait, run)
	return func(kind string, waited, ran time.Duration, err error) {
		result := "done"
		if err != nil {
			result = "failed"
		}
		ended.WithLabelValues(kind, result).Inc()
		wait.Observe(waited.Seconds())
		run.WithLabelValues(kind).Observe(ran.Seconds())
	}
}
