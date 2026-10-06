package ratelimit

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"

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

// Proxies are the addresses our own load balancers connect from.
type Proxies []netip.Prefix

// has says if addr is one of ours.
func (ps Proxies) has(addr string) bool {
	ip, err := netip.ParseAddr(addr)
	inside := func(p netip.Prefix) bool { return p.Contains(ip.Unmap()) }
	return err == nil && slices.ContainsFunc(ps, inside)
}

// ByIP is the client's address, for an API behind proxies of its own.
func (ps Proxies) ByIP(r *http.Request) string {
	peer := ByIP(r)
	if !ps.has(peer) {
		return peer
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for _, hop := range slices.Backward(hops) {
		if hop = strings.TrimSpace(hop); hop != "" && !ps.has(hop) {
			return hop
		}
	}
	return peer
}

// An Allower takes one token from a key's bucket, wherever the buckets live. It can fail.
type Allower interface {
	Allow(ctx context.Context, key string) (Result, error)
	Policy() (name string, perMinute int)
}

// Limit lets a request through to next when its bucket has a token.
func Limit(l Allower, key Key, next http.HandlerFunc) http.HandlerFunc {
	name, perMinute := l.Policy()
	policy := fmt.Sprintf("%q;q=%d;w=60", name, perMinute)
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := l.Allow(r.Context(), key(r))
		if err != nil {
			httpx.Error(w, http.StatusServiceUnavailable, "the rate limiter is not answering")
			return
		}
		w.Header().Add("RateLimit-Policy", policy)
		w.Header().Add("RateLimit", fmt.Sprintf("%q;a=%d", name, res.Remaining))
		if !res.Allowed {
			wait := math.Ceil(res.RetryAfter.Seconds())
			w.Header().Set("Retry-After", strconv.Itoa(int(wait)))
			httpx.Error(w, http.StatusTooManyRequests, "too many requests")
			return
		}
		next(w, r)
	}
}
