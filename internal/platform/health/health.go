// Package health answers two questions about a process: is it up, and can it
// do its job right now.
package health

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

// A Check asks one thing this process can't work without, such as its database.
type Check func(ctx context.Context) error

// Health holds the answers of one process.
type Health struct {
	checks  []Check
	leaving atomic.Bool
}

func New(checks ...Check) *Health {
	return &Health{checks: checks}
}

// Routes puts both answers on a mux.
func (h *Health) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", h.Live)
	mux.HandleFunc("GET /readyz", h.Ready)
}

// Live says the process is up. It asks nobody, so it can't be slow.
func (h *Health) Live(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "ok")
}

// Ready says this process can do its job right now.
func (h *Health) Ready(w http.ResponseWriter, r *http.Request) {
	if h.leaving.Load() {
		http.Error(w, "shutting down", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	for _, check := range h.checks {
		if err := check(ctx); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
	}
	fmt.Fprintln(w, "ready")
}

// Leave is the first step of a shutdown. From here on, Ready says no.
func (h *Health) Leave() {
	h.leaving.Store(true)
}

// Probe asks the process on this machine's private port if it is ready.
// A container without curl checks its own health with it.
func Probe(addr string) error {
	client := http.Client{Timeout: 2 * time.Second}
	res, err := client.Get("http://localhost" + addr + "/readyz")
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("not ready: %s", res.Status)
	}
	return nil
}
