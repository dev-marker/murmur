package ratelimit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

// Requests go through the middleware, and we read the status and the headers of every answer.
func TestLimit(t *testing.T) {
	l, clock := limiter(2, 30) // two at once, and then one every two seconds
	key := func(*http.Request) string { return "ann" }
	h := Limit(l, key, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	steps := []struct {
		wait   time.Duration
		status int
		left   string // the RateLimit header
		retry  string // the Retry-After header
	}{
		{0, 204, `"test";a=1`, ""},
		{0, 204, `"test";a=0`, ""},
		{0, 429, `"test";a=0`, "2"},
		{500 * time.Millisecond, 429, `"test";a=0`, "2"}, // a second and a half, rounded up
		{time.Second, 429, `"test";a=0`, "1"},
		{500 * time.Millisecond, 204, `"test";a=0`, ""},
	}
	for i, s := range steps {
		*clock = clock.Add(s.wait)
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest("GET", "/feed", nil))
		head := rec.Header()
		if rec.Code != s.status || head.Get("RateLimit") != s.left || head.Get("Retry-After") != s.retry {
			t.Fatalf("step %d: status %d, RateLimit %q, Retry-After %q", i+1,
				rec.Code, head.Get("RateLimit"), head.Get("Retry-After"))
		}
		if got := head.Get("RateLimit-Policy"); got != `"test";q=30;w=60` {
			t.Fatalf("step %d: RateLimit-Policy %q", i+1, got)
		}
	}
}

func TestByIP(t *testing.T) {
	for addr, want := range map[string]string{
		"203.0.113.7:51234":   "203.0.113.7",
		"[2001:db8::7]:51234": "2001:db8::7",
	} {
		r := httptest.NewRequest("GET", "/feed", nil)
		r.RemoteAddr = addr
		if got := ByIP(r); got != want {
			t.Errorf("ByIP(%q) = %q, want %q", addr, got, want)
		}
	}
}

// Who is the client? It depends on who we hear it from.
func TestProxiesByIP(t *testing.T) {
	ours := Proxies{netip.MustParsePrefix("127.0.0.1/32"), netip.MustParsePrefix("10.0.0.0/8")}
	tests := []struct {
		name      string
		peer      string // where the connection comes from
		forwarded string // the X-Forwarded-For header
		want      string
	}{
		{"a stranger is never believed", "198.51.100.9:4000", "203.0.113.7", "198.51.100.9"},
		{"our proxy is", "127.0.0.1:4000", "203.0.113.7", "203.0.113.7"},
		{"what the client wrote is ignored", "127.0.0.1:4000", "1.2.3.4, 203.0.113.7", "203.0.113.7"},
		{"two proxies of ours in a row", "127.0.0.1:4000", "1.2.3.4, 203.0.113.7, 10.1.2.3", "203.0.113.7"},
		{"an IPv6 client", "127.0.0.1:4000", "2001:db8::7", "2001:db8::7"},
		{"our proxy without the header", "127.0.0.1:4000", "", "127.0.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/feed", nil)
			r.RemoteAddr = tt.peer
			if tt.forwarded != "" {
				r.Header.Set("X-Forwarded-For", tt.forwarded)
			}
			if got := ours.ByIP(r); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// away is a limiter that can't answer, like Redis when it is stopped.
type away struct{ Allower }

func (away) Allow(context.Context, string) (Result, error) {
	return Result{}, errors.New("redis is away")
}

// A limiter that can't answer, and has nothing to fall back on, lets nobody in.
func TestLimitFailsClosed(t *testing.T) {
	l, _ := limiter(2, 30)
	h := Limit(away{l}, ByIP, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("the request got through")
	})
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest("GET", "/feed", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", rec.Code)
	}
}
