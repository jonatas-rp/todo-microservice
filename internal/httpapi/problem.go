package httpapi

import (
	"errors"
	"net/http"

	"todo-service/internal/logger"
	"todo-service/internal/todo"
)

// problem is an RFC 9457 problem details body.
type problem struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

func writeProblem(w http.ResponseWriter, r *http.Request, status int, detail string) {
	logger := logger.FromContext(r.Context())
	logger.Error(detail, map[string]any{
		"status": status,
		"method": r.Method,
		"path":   r.URL.Path,
	})
	writeJSON(w, status, contentTypeProblem, problem{
		Type:     "about:blank",
		Title:    http.StatusText(status),
		Status:   status,
		Detail:   detail,
		Instance: r.URL.Path,
	})
}

func writeDecodeError(w http.ResponseWriter, r *http.Request, err error) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		writeProblem(w, r, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	writeProblem(w, r, http.StatusBadRequest, "request body must be valid JSON with only title and completed")
}

func writeTodoError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, todo.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "todo not found")
	case errors.Is(err, todo.ErrEmptyTitle):
		writeProblem(w, r, http.StatusUnprocessableEntity, "title is required")
	case errors.Is(err, todo.ErrIdempotencyKey):
		writeProblem(w, r, http.StatusBadRequest, "Idempotency-Key must be a UUID")
	default:
		writeProblem(w, r, http.StatusInternalServerError, "internal error")
	}
}
