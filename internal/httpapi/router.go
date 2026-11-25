package httpapi

import (
	"net/http"

	"github.com/swaggest/swgui/v5emb"

	"todo-service/internal/ratelimit"
)

// NewHandler serves the to-do API plus liveness and readiness probes.
func NewHandler(svc TodoService, ping Pinger, lim *ratelimit.Limiter, readiness *Readiness) http.Handler {
	mux := http.NewServeMux()
	h := &todoHandler{svc: svc}
	mux.HandleFunc("GET /v1/todos", h.list)
	mux.HandleFunc("POST /v1/todos", h.create)
	mux.HandleFunc("PUT /v1/todos/{id}", h.update)
	mux.HandleFunc("DELETE /v1/todos/{id}", h.remove)
	mux.HandleFunc("GET /v1/openapi.json", openAPI)
	mux.Handle("/v1/docs/", v5emb.New("To-do service", "/v1/openapi.json", "/v1/docs/"))
	mux.HandleFunc("GET /livez", live)
	mux.HandleFunc("GET /readyz", ready(ping, readiness))
	mux.HandleFunc("GET /health", health)

	handler := problemFallback(mux)
	handler = recoverPanic(handler)
	handler = rateLimit(lim, handler)
	handler = requestLog(handler)
	return handler
}
