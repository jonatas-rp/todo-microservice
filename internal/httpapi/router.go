package httpapi

import (
	"net/http"

	"github.com/swaggest/swgui"
	"github.com/swaggest/swgui/v5emb"

	"todo-service/internal/ratelimit"
)

// NewHandler serves the to-do API plus liveness and readiness probes.
// publicPrefix is the ingress path in front of the API, such as /dev. It is
// empty when clients call this process directly. The docs UI is always mounted
// at /v1/docs/; the prefix is only written into the URLs the browser requests.
func NewHandler(svc TodoService, ping Pinger, lim *ratelimit.Limiter, readiness *Readiness, publicPrefix string) http.Handler {
	mux := http.NewServeMux()
	h := &todoHandler{svc: svc}
	mux.HandleFunc("GET /v1/todos", h.list)
	mux.HandleFunc("POST /v1/todos", h.create)
	mux.HandleFunc("PUT /v1/todos/{id}", h.update)
	mux.HandleFunc("DELETE /v1/todos/{id}", h.remove)
	mux.HandleFunc("GET /v1/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		openAPI(w, r, publicPrefix)
	})
	specPath := publicPrefix + "/v1/openapi.json"
	docsPath := publicPrefix + "/v1/docs/"
	mux.Handle("/v1/docs/", v5emb.NewHandlerWithConfig(swgui.Config{
		Title:            "To-do service",
		SwaggerJSON:      specPath,
		BasePath:         docsPath,
		InternalBasePath: "/v1/docs/",
	}))
	mux.HandleFunc("GET /livez", live)
	mux.HandleFunc("GET /readyz", ready(ping, readiness))
	mux.HandleFunc("GET /health", health)

	handler := problemFallback(mux)
	handler = recoverPanic(handler)
	handler = rateLimit(lim, handler)
	handler = requestLog(handler)
	return handler
}
