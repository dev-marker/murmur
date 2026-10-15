package metrics

import (
	"cmp"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// HTTP counts the requests a server answers, and how long they took.
type HTTP struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func NewHTTP(reg prometheus.Registerer) *HTTP {
	m := &HTTP{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Requests answered, by route and status.",
		}, []string{"route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "How long a request took, by route.",
			Buckets: []float64{.005, .01, .015, .02, .03, .04, .06, .08, .1, .15, .25, .5, 1, 2.5, 10},
		}, []string{"route"}),
	}
	reg.MustRegister(m.requests, m.duration)
	return m
}

// A recorder remembers the status code that a handler sends.
type recorder struct {
	http.ResponseWriter
	status int
}

func (rec *recorder) WriteHeader(status int) {
	rec.status = status
	rec.ResponseWriter.WriteHeader(status)
}

// Unwrap hands out the real writer. http.ResponseController asks for it,
// when a handler wants to flush or to set a deadline.
func (rec *recorder) Unwrap() http.ResponseWriter {
	return rec.ResponseWriter
}

// Wrap measures every request that goes through next.
func (m *HTTP) Wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &recorder{ResponseWriter: w, status: http.StatusOK}
		next(rec, r)
		route := cmp.Or(r.Pattern, "other")
		m.requests.WithLabelValues(route, strconv.Itoa(rec.status)).Inc()
		if rec.Header().Get("Content-Type") == "text/event-stream" {
			return // a stream is open for minutes: it's counted, but it's not a slow request
		}
		m.duration.WithLabelValues(route).Observe(time.Since(start).Seconds())
	}
}
