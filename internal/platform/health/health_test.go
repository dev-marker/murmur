package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Ready follows what the process needs, and it says no for good once we leave.
// Live says yes through all of it.
func TestReady(t *testing.T) {
	var postgres error
	h := New(func(context.Context) error { return postgres })
	mux := http.NewServeMux()
	h.Routes(mux)
	ask := func(path string) int {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec.Code
	}
	for _, step := range []struct {
		name  string
		do    func()
		ready int
	}{
		{"at the start", func() {}, 200},
		{"postgres is away", func() { postgres = errors.New("no answer") }, 503},
		{"postgres is back", func() { postgres = nil }, 200},
		{"we are leaving", h.Leave, 503},
	} {
		step.do()
		if got := ask("/readyz"); got != step.ready {
			t.Errorf("%s: ready says %d, want %d", step.name, got, step.ready)
		}
		if got := ask("/healthz"); got != 200 {
			t.Errorf("%s: live says %d, want 200", step.name, got)
		}
	}
}
