package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"todo-service/internal/ratelimit"
	"todo-service/internal/storage"
)

func TestRateLimit(t *testing.T) {
	lim := ratelimit.New(storage.NewMemory(), 1, 0)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /todos", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /livez", live)
	h := rateLimit(lim, mux)

	call := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "203.0.113.7:1234"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	if rec := call("/todos"); rec.Code != http.StatusNoContent {
		t.Fatalf("first = %d", rec.Code)
	}
	rec := call("/todos")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Content-Type") != contentTypeProblem {
		t.Fatalf("second = %d %q %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "rate limit exceeded") {
		t.Fatalf("body = %s", rec.Body)
	}
	if rec := call("/livez"); rec.Code != http.StatusOK {
		t.Fatalf("livez = %d %s", rec.Code, rec.Body)
	}
}

func TestRateLimitSkipsProbesAndDocs(t *testing.T) {
	lim := ratelimit.New(storage.NewMemory(), 1, 0)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("GET /readyz", ready(nil, NewReadiness()))
	mux.HandleFunc("GET /v1/docs/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /todos", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := rateLimit(lim, mux)

	call := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "203.0.113.7:1234"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	if rec := call("/todos"); rec.Code != http.StatusNoContent {
		t.Fatalf("first todos = %d", rec.Code)
	}
	if rec := call("/readyz"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz = %d %s", rec.Code, rec.Body)
	}
	if rec := call("/v1/docs/"); rec.Code != http.StatusNoContent {
		t.Fatalf("docs = %d", rec.Code)
	}
	if rec := call("/health"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("health = %d %s", rec.Code, rec.Body)
	}
}

func TestReadyNilPinger(t *testing.T) {
	rec := httptest.NewRecorder()
	ready(nil, NewReadiness()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "storage is not configured") {
		t.Fatalf("body = %s", rec.Body)
	}
}

func TestRequestLog(t *testing.T) {
	var buf bytes.Buffer
	prev := requestLogWriter
	requestLogWriter = &buf
	t.Cleanup(func() { requestLogWriter = prev })

	mux := http.NewServeMux()
	mux.HandleFunc("GET /todos", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := requestLog(mux)

	var ids []string
	for range 2 {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/todos", nil))
		ids = append(ids, rec.Header().Get("X-Request-ID"))
		if rec.Code != http.StatusNoContent || len(ids[len(ids)-1]) != 36 {
			t.Fatalf("status %d id %q", rec.Code, ids[len(ids)-1])
		}
	}
	if ids[0] == ids[1] {
		t.Fatalf("request ids repeated: %s", ids[0])
	}

	var lines []struct {
		RequestID string         `json:"request_id"`
		Message   string         `json:"message"`
		Context   map[string]any `json:"context"`
	}
	for _, raw := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var line struct {
			RequestID string         `json:"request_id"`
			Message   string         `json:"message"`
			Context   map[string]any `json:"context"`
		}
		if err := json.Unmarshal([]byte(raw), &line); err != nil {
			t.Fatalf("decode %s: %v", raw, err)
		}
		lines = append(lines, line)
	}
	if len(lines) != 2 {
		t.Fatalf("lines = %d, body %s", len(lines), buf.String())
	}
	for i, line := range lines {
		if line.Message != "request received" || line.RequestID != ids[i] || line.Context["method"] != "GET" || line.Context["path"] != "/todos" {
			t.Fatalf("line %d = %+v, id %s", i, line, ids[i])
		}
	}
}

func TestRecoverPanic(t *testing.T) {
	h := recoverPanic(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/todos", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != contentTypeProblem {
		t.Fatalf("Content-Type = %q", ct)
	}
	var p problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Status != http.StatusInternalServerError || p.Detail != "internal error" || p.Instance != "/todos" {
		t.Fatalf("problem = %+v", p)
	}
}

func TestRecoverPanicAfterHeaderWritten(t *testing.T) {
	h := recoverPanic(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusAccepted || rec.Body.Len() != 0 {
		t.Fatalf("status = %d body %q", rec.Code, rec.Body)
	}
}

func TestRecoverPanicReraisesAbort(t *testing.T) {
	h := recoverPanic(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	defer func() {
		if v := recover(); v != http.ErrAbortHandler {
			t.Fatalf("recovered %v, want http.ErrAbortHandler", v)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}
