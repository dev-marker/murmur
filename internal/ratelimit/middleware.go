package ratelimit

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"

	"github.com/fiston-user/murmur/internal/platform/httpx"
)

// A Key says whose bucket a request takes its token from.
type Key func(r *http.Request) string

// ByIP is the address a request came from, without the port.
func ByIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Limit lets a request through to next when its bucket has a token.
func (l *Limiter) Limit(key Key, next http.HandlerFunc) http.HandlerFunc {
	policy := fmt.Sprintf("%q;q=%d;w=60", l.name, l.perMinute)
	return func(w http.ResponseWriter, r *http.Request) {
		res := l.Allow(key(r))
		w.Header().Add("RateLimit-Policy", policy)
		w.Header().Add("RateLimit", fmt.Sprintf("%q;a=%d", l.name, res.Remaining))
		if !res.Allowed {
			wait := math.Ceil(res.RetryAfter.Seconds())
			w.Header().Set("Retry-After", strconv.Itoa(int(wait)))
			httpx.Error(w, http.StatusTooManyRequests, "too many requests")
			return
		}
		next(w, r)
	}
}
