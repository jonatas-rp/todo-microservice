package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"todo-service/internal/httpapi"
	"todo-service/internal/ratelimit"
	"todo-service/internal/storage"
	"todo-service/internal/todo"
)

type apiTodo struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
}

type envelope[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type problem struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Instance string `json:"instance"`
}

func TestTodoAPI(t *testing.T) {
	store := openStore(t)
	handler := httpapi.NewHandler(todo.New(store), store, allowAll(), httpapi.NewReadiness())

	for _, path := range []string{"/livez", "/readyz", "/health"} {
		rec := request(t, handler, http.MethodGet, path, "")
		env := decodeSuccess[map[string]string](t, rec, http.StatusOK)
		if env.Data["status"] == "" {
			t.Fatalf("%s data = %+v", path, env.Data)
		}
	}

	rec := request(t, handler, http.MethodGet, "/v1/todos", "")
	empty := decodeSuccess[[]apiTodo](t, rec, http.StatusOK)
	if empty.Data == nil || len(empty.Data) != 0 || empty.Message != "todos retrieved" {
		t.Fatalf("empty list = %+v", empty)
	}

	rec = request(t, handler, http.MethodPost, "/v1/todos", `{"title":"  buy milk  ","completed":false}`)
	created := decodeSuccess[apiTodo](t, rec, http.StatusCreated).Data
	if created.Title != "buy milk" || created.Completed || created.ID == "" {
		t.Fatalf("created = %+v", created)
	}
	if rec.Header().Get("Location") != "/v1/todos/"+created.ID {
		t.Fatalf("Location = %q", rec.Header().Get("Location"))
	}

	rec = request(t, handler, http.MethodGet, "/v1/todos", "")
	listed := decodeSuccess[[]apiTodo](t, rec, http.StatusOK).Data
	if len(listed) != 1 || listed[0] != created {
		t.Fatalf("list = %+v", listed)
	}

	rec = request(t, handler, http.MethodPut, "/v1/todos/"+created.ID, `{"title":"buy oats","completed":true}`)
	updated := decodeSuccess[apiTodo](t, rec, http.StatusOK).Data
	if updated.ID != created.ID || updated.Title != "buy oats" || !updated.Completed {
		t.Fatalf("update = %+v", updated)
	}

	rec = request(t, handler, http.MethodDelete, "/v1/todos/"+created.ID, "")
	deleted := decodeSuccess[*apiTodo](t, rec, http.StatusOK)
	if deleted.Message != "todo deleted" || deleted.Data != nil {
		t.Fatalf("delete = %+v", deleted)
	}
	if !strings.Contains(rec.Body.String(), `"data":null`) {
		t.Fatalf("delete body = %s", rec.Body)
	}

	rec = request(t, handler, http.MethodGet, "/v1/todos", "")
	if got := decodeSuccess[[]apiTodo](t, rec, http.StatusOK).Data; len(got) != 0 {
		t.Fatalf("list after delete = %+v", got)
	}
}

func TestIdempotencyKey(t *testing.T) {
	store := openStore(t)
	handler := httpapi.NewHandler(todo.New(store), store, allowAll(), httpapi.NewReadiness())

	rec := requestWithKey(t, handler, http.MethodPost, "/v1/todos", `{"title":"buy milk"}`, "")
	if p := decodeProblem(t, rec, http.StatusBadRequest); p.Detail != "Idempotency-Key header is required" {
		t.Fatalf("missing key = %+v", p)
	}

	rec = requestWithKey(t, handler, http.MethodPost, "/v1/todos", `{"title":"buy milk","completed":false}`, "not-a-uuid")
	if p := decodeProblem(t, rec, http.StatusBadRequest); p.Detail != "Idempotency-Key must be a UUID" {
		t.Fatalf("invalid key = %+v", p)
	}

	const key = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	first := requestWithKey(t, handler, http.MethodPost, "/v1/todos", `{"title":"buy milk","completed":false}`, key)
	created := decodeSuccess[apiTodo](t, first, http.StatusCreated).Data

	second := requestWithKey(t, handler, http.MethodPost, "/v1/todos", `{"title":"other","completed":true}`, key)
	replay := decodeSuccess[apiTodo](t, second, http.StatusOK)
	if replay.Message != "todo already created" || replay.Data != created {
		t.Fatalf("replay = %+v, created %+v", replay, created)
	}

	listed := decodeSuccess[[]apiTodo](t, request(t, handler, http.MethodGet, "/v1/todos", ""), http.StatusOK).Data
	if len(listed) != 1 || listed[0] != created {
		t.Fatalf("list = %+v", listed)
	}
}

func TestTodoAPIErrors(t *testing.T) {
	store := openStore(t)
	handler := httpapi.NewHandler(todo.New(store), store, allowAll(), httpapi.NewReadiness())

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		status int
		allow  string
	}{
		{"blank title", http.MethodPost, "/v1/todos", `{"title":"   "}`, http.StatusUnprocessableEntity, ""},
		{"malformed json", http.MethodPost, "/v1/todos", `{`, http.StatusBadRequest, ""},
		{"unknown field", http.MethodPost, "/v1/todos", `{"title":"x","extra":1}`, http.StatusBadRequest, ""},
		{"update missing", http.MethodPut, "/v1/todos/missing", `{"title":"x","completed":false}`, http.StatusNotFound, ""},
		{"delete missing", http.MethodDelete, "/v1/todos/missing", "", http.StatusNotFound, ""},
		{"unknown route", http.MethodGet, "/nope", "", http.StatusNotFound, ""},
		{"method not allowed", http.MethodPatch, "/v1/todos", "", http.StatusMethodNotAllowed, "GET, HEAD, POST"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := request(t, handler, tt.method, tt.path, tt.body)
			p := decodeProblem(t, rec, tt.status)
			if p.Instance != tt.path {
				t.Fatalf("instance = %q, want %q", p.Instance, tt.path)
			}
			if tt.allow != "" && rec.Header().Get("Allow") != tt.allow {
				t.Fatalf("Allow = %q, want %q", rec.Header().Get("Allow"), tt.allow)
			}
		})
	}
}

func TestOpenAPI(t *testing.T) {
	handler := httpapi.NewHandler(todo.New(openStore(t)), nil, allowAll(), httpapi.NewReadiness())
	rec := request(t, handler, http.MethodGet, "/v1/openapi.json", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q", ct)
	}
	want, err := os.ReadFile(filepath.Join("..", "..", "docs", "openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rec.Body.Bytes(), want) {
		t.Fatalf("body = %s", rec.Body)
	}

	ui := request(t, handler, http.MethodGet, "/v1/docs/", "")
	if ui.Code != http.StatusOK {
		t.Fatalf("docs status = %d, body %s", ui.Code, ui.Body)
	}
	if ct := ui.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("docs Content-Type = %q", ct)
	}
	if !strings.Contains(ui.Body.String(), "swagger-ui") || !strings.Contains(ui.Body.String(), "/v1/openapi.json") {
		t.Fatalf("docs body = %s", ui.Body)
	}
}

func TestReadyzWhenClosed(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "todos.db"))
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewHandler(todo.New(store), store, allowAll(), httpapi.NewReadiness())
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	rec := request(t, handler, http.MethodGet, "/readyz", "")
	decodeProblem(t, rec, http.StatusServiceUnavailable)
}

func request(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return requestWithKey(t, h, method, path, body, newKey())
}

func requestWithKey(t *testing.T, h http.Handler, method, path, body, key string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if method == http.MethodPost && key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func newKey() string {
	return uuid.NewString()
}

func decodeSuccess[T any](t *testing.T, rec *httptest.ResponseRecorder, status int) envelope[T] {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body %s", rec.Code, status, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q", ct)
	}
	var env envelope[T]
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	if !env.Success || env.Message == "" {
		t.Fatalf("envelope = %+v", env)
	}
	return env
}

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder, status int) problem {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body %s", rec.Code, status, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("Content-Type = %q; body %s", ct, rec.Body)
	}
	var p problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	if p.Type != "about:blank" || p.Status != status || p.Title != http.StatusText(status) || p.Detail == "" {
		t.Fatalf("problem = %+v", p)
	}
	return p
}

func allowAll() *ratelimit.Limiter {
	return ratelimit.New(storage.NewMemory(), 1_000_000, 1_000_000)
}

func openStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "todos.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
