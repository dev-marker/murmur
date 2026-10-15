package metrics

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// A Queue reads how much work is waiting, from Postgres, when Prometheus asks.
type Queue struct {
	db     *pgxpool.Pool
	jobs   *prometheus.Desc
	oldest *prometheus.Desc
	deltas *prometheus.Desc
}

func NewQueue(db *pgxpool.Pool) *Queue {
	return &Queue{
		db:     db,
		jobs:   prometheus.NewDesc("murmur_queue_jobs", "Jobs in the queue.", []string{"status"}, nil),
		oldest: prometheus.NewDesc("murmur_queue_oldest_seconds", "The oldest job's wait.", nil, nil),
		deltas: prometheus.NewDesc("murmur_like_deltas", "Likes not yet in a post's count.", nil, nil),
	}
}

func (q *Queue) Describe(ch chan<- *prometheus.Desc) {
	ch <- q.jobs
	ch <- q.oldest
	ch <- q.deltas
}

func (q *Queue) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var waiting, running, dead, oldest, deltas float64
	err := q.db.QueryRow(ctx, `
		select count(*) filter (where status = 'waiting'),
			count(*) filter (where status = 'running'),
			count(*) filter (where status = 'dead'),
			coalesce(extract(epoch from now() - min(created_at) filter (where status <> 'dead')), 0),
			(select count(*) from like_deltas)
		from jobs`).Scan(&waiting, &running, &dead, &oldest, &deltas)
	if err != nil {
		slog.Error("the queue was not measured", "error", err)
		return
	}
	gauge := func(d *prometheus.Desc, v float64, labels ...string) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v, labels...)
	}
	gauge(q.jobs, waiting, "waiting")
	gauge(q.jobs, running, "running")
	gauge(q.jobs, dead, "dead")
	gauge(q.oldest, oldest)
	gauge(q.deltas, deltas)
}
