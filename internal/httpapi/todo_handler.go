package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"todo-service/internal/logger"
	"todo-service/internal/todo"
)

const idempotencyKeyHeader = "Idempotency-Key"

// TodoService is the use-case port the HTTP adapter calls.
type TodoService interface {
	List(ctx context.Context) ([]todo.Todo, error)
	Create(ctx context.Context, title string, completed bool, idempotencyKey string) (todo.Todo, bool, error)
	Update(ctx context.Context, id, title string, completed bool) (todo.Todo, error)
	Delete(ctx context.Context, id string) error
}

type todoHandler struct {
	svc TodoService
}

type todoRequest struct {
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
}

func (h *todoHandler) list(w http.ResponseWriter, r *http.Request) {
	logger := logger.FromContext(r.Context())

	items, err := h.svc.List(r.Context())
	if err != nil {
		writeTodoError(w, r, err)
		return
	}

	logger.Info("todos retrieved", map[string]any{
		"status": http.StatusOK,
		"count":  len(items),
	})
	writeSuccess(w, http.StatusOK, "todos retrieved", items)
}

func (h *todoHandler) create(w http.ResponseWriter, r *http.Request) {
	logger := logger.FromContext(r.Context())

	key := strings.TrimSpace(r.Header.Get(idempotencyKeyHeader))
	if key == "" {
		writeProblem(w, r, http.StatusBadRequest, "Idempotency-Key header is required")
		return
	}
	body, ok := readTodoRequest(w, r)
	if !ok {
		return
	}
	item, created, err := h.svc.Create(r.Context(), body.Title, body.Completed, key)
	if err != nil {
		writeTodoError(w, r, err)
		return
	}
	status := http.StatusOK
	message := "todo already created"
	if created {
		status = http.StatusCreated
		message = "todo created"
	}

	logger.Info(message, map[string]any{
		"status": status,
		"id":     item.PublicID,
	})
	w.Header().Set("Location", "/v1/todos/"+url.PathEscape(item.PublicID))
	writeSuccess(w, status, message, item)
}

func (h *todoHandler) update(w http.ResponseWriter, r *http.Request) {
	logger := logger.FromContext(r.Context())

	body, ok := readTodoRequest(w, r)
	if !ok {
		return
	}
	item, err := h.svc.Update(r.Context(), r.PathValue("id"), body.Title, body.Completed)
	if err != nil {
		writeTodoError(w, r, err)
		return
	}

	logger.Info("todo updated", map[string]any{
		"status": http.StatusOK,
		"id":     item.PublicID,
	})
	writeSuccess(w, http.StatusOK, "todo updated", item)
}

func (h *todoHandler) remove(w http.ResponseWriter, r *http.Request) {
	logger := logger.FromContext(r.Context())

	id := r.PathValue("id")
	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeTodoError(w, r, err)
		return
	}

	logger.Info("todo deleted", map[string]any{
		"status": http.StatusOK,
		"id":     id,
	})
	writeSuccess(w, http.StatusOK, "todo deleted", nil)
}

func readTodoRequest(w http.ResponseWriter, r *http.Request) (todoRequest, bool) {
	var body todoRequest
	if err := readJSON(r, &body); err != nil {
		writeDecodeError(w, r, err)
		return todoRequest{}, false
	}
	return body, true
}
