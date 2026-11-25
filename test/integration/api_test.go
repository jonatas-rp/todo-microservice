package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestCRUDAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "todos.db")
	handler, store := newHandler(t, path, 1_000_000, 1_000_000)

	key := uuid.NewString()
	rec := request(t, handler, http.MethodPost, "/v1/todos", `{"title":"buy milk","completed":false}`, key)
	created := decodeSuccess[apiTodo](t, rec, http.StatusCreated).Data
	if created.Title != "buy milk" || created.ID == "" {
		t.Fatalf("created = %+v", created)
	}
	if id := rec.Header().Get("X-Request-ID"); id == "" {
		t.Fatal("missing X-Request-ID on create")
	}

	listed := decodeSuccess[[]apiTodo](t, request(t, handler, http.MethodGet, "/v1/todos", "", ""), http.StatusOK).Data
	if len(listed) != 1 || listed[0] != created {
		t.Fatalf("list = %+v", listed)
	}

	updated := decodeSuccess[apiTodo](t, request(t, handler, http.MethodPut, "/v1/todos/"+created.ID, `{"title":"buy oats","completed":true}`, ""), http.StatusOK).Data
	if updated.ID != created.ID || updated.Title != "buy oats" || !updated.Completed {
		t.Fatalf("updated = %+v", updated)
	}

	replay := decodeSuccess[apiTodo](t, request(t, handler, http.MethodPost, "/v1/todos", `{"title":"other","completed":true}`, key), http.StatusOK)
	if replay.Message != "todo already created" || replay.Data.ID != created.ID {
		t.Fatalf("replay = %+v", replay)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	handler, store = newHandler(t, path, 1_000_000, 1_000_000)
	t.Cleanup(func() { _ = store.Close() })

	again := decodeSuccess[[]apiTodo](t, request(t, handler, http.MethodGet, "/v1/todos", "", ""), http.StatusOK).Data
	if len(again) != 1 || again[0].ID != created.ID || again[0].Title != "buy oats" {
		t.Fatalf("reopened list = %+v", again)
	}

	deleted := decodeSuccess[*apiTodo](t, request(t, handler, http.MethodDelete, "/v1/todos/"+created.ID, "", ""), http.StatusOK)
	if deleted.Message != "todo deleted" || deleted.Data != nil {
		t.Fatalf("delete = %+v", deleted)
	}
}

func TestRateLimitAndSkips(t *testing.T) {
	handler, _ := newHandler(t, filepath.Join(t.TempDir(), "todos.db"), 2, 0)

	for range 2 {
		rec := request(t, handler, http.MethodGet, "/v1/todos", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("allowed = %d %s", rec.Code, rec.Body)
		}
	}
	rec := request(t, handler, http.MethodGet, "/v1/todos", "", "")
	p := decodeProblem(t, rec, http.StatusTooManyRequests)
	if p.Detail != "rate limit exceeded" {
		t.Fatalf("problem = %+v", p)
	}
	if id := rec.Header().Get("X-Request-ID"); id == "" {
		t.Fatal("missing X-Request-ID on 429")
	}

	if rec := request(t, handler, http.MethodGet, "/livez", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("livez = %d", rec.Code)
	}
	if rec := request(t, handler, http.MethodGet, "/v1/docs/", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("docs = %d", rec.Code)
	}
}

func TestBodyTooLarge(t *testing.T) {
	handler, _ := newHandler(t, filepath.Join(t.TempDir(), "todos.db"), 1_000_000, 1_000_000)
	body := `{"title":"` + strings.Repeat("a", 1<<20) + `","completed":false}`
	rec := request(t, handler, http.MethodPost, "/v1/todos", body, uuid.NewString())
	p := decodeProblem(t, rec, http.StatusRequestEntityTooLarge)
	if p.Detail != "request body too large" {
		t.Fatalf("problem = %+v", p)
	}
}

func newHandler(t *testing.T, path string, capacity, perSecond float64) (http.Handler, *storage.Store) {
	t.Helper()
	store, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	lim := ratelimit.New(storage.NewMemory(), capacity, perSecond)
	return httpapi.NewHandler(todo.New(store), store, lim, httpapi.NewReadiness()), store
}

func request(t *testing.T, h http.Handler, method, path, body, key string) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	req.RemoteAddr = "203.0.113.10:4321"
	if method == http.MethodPost && key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
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
	if p.Type != "about:blank" || p.Status != status || p.Detail == "" {
		t.Fatalf("problem = %+v", p)
	}
	return p
}
