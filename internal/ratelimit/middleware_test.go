package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Requests go through the middleware, and we read the status and the headers of every answer.
func TestLimit(t *testing.T) {
	l, clock := limiter(2, 30) // two at once, and then one every two seconds
	key := func(*http.Request) string { return "ann" }
	h := l.Limit(key, func(w http.ResponseWriter, r *http.Request) {
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
