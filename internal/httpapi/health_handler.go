// Package httpapi serves the to-do HTTP API, the health probes, and the docs UI.
package httpapi

import (
	"context"
	"net/http"
	"sync"

	"todo-service/internal/logger"
)

// Pinger reports whether a dependency can serve traffic.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Readiness records whether this process should receive traffic.
type Readiness struct {
	mu    sync.RWMutex
	ready bool
}

type healthStatus struct {
	Status string `json:"status"`
}

// NewReadiness returns a Readiness that starts ready.
func NewReadiness() *Readiness {
	return &Readiness{ready: true}
}

// SetReadiness marks the process ready or not ready.
func (r *Readiness) SetReadiness(ready bool) {
	r.mu.Lock()
	r.ready = ready
	r.mu.Unlock()
}

// GetReadiness reports whether the process should receive traffic.
func (r *Readiness) GetReadiness() bool {
	r.mu.RLock()
	ready := r.ready
	r.mu.RUnlock()
	return ready
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeSuccess(w, http.StatusOK, "service is healthy", healthStatus{Status: "healthy"})
}

func live(w http.ResponseWriter, _ *http.Request) {
	writeSuccess(w, http.StatusOK, "service is live", healthStatus{Status: "live"})
}

func ready(ping Pinger, readiness *Readiness) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ping == nil {
			writeProblem(w, r, http.StatusServiceUnavailable, "storage is not configured")
			return
		}
		if !readiness.GetReadiness() {
			writeProblem(w, r, http.StatusServiceUnavailable, "service is not ready")
			return
		}
		if err := ping.Ping(r.Context()); err != nil {
			logger.FromContext(r.Context()).Error("readiness check failed", map[string]any{
				"err": err.Error(),
			})
			writeProblem(w, r, http.StatusServiceUnavailable, "storage is unavailable")
			return
		}
		writeSuccess(w, http.StatusOK, "service is ready", healthStatus{Status: "ready"})
	}
}
