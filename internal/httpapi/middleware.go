package httpapi

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"

	"todo-service/internal/logger"
	"todo-service/internal/ratelimit"
)

// requestLogWriter is where request lines are written. Tests replace it.
var requestLogWriter io.Writer = os.Stdout

// requestLog stores a fresh logger in the request context and logs the route.
// The logger is not kept anywhere else, so the next request cannot see it.
func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.NewString()
		l := logger.New(requestLogWriter, id)
		r = r.WithContext(logger.NewContext(r.Context(), l))
		w.Header().Set("X-Request-ID", id)

		if r.URL.Path != "/livez" && r.URL.Path != "/readyz" {
			l.Info("request received", map[string]any{
				"method": r.Method,
				"path":   r.URL.Path,
			})
		}
		next.ServeHTTP(w, r)
	})
}

// rateLimit rejects clients that have spent their token bucket.
// Liveness, readiness, and the documentation UI are not counted.
func rateLimit(lim *ratelimit.Limiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := logger.FromContext(r.Context())
		if r.URL.Path == "/livez" || r.URL.Path == "/readyz" || strings.HasPrefix(r.URL.Path, "/v1/docs") {
			next.ServeHTTP(w, r)
			return
		}
		if !lim.Allow(clientIP(r), time.Now()) {
			logger.Error("rate limit exceeded", map[string]any{
				"method": r.Method,
				"path":   r.URL.Path,
			})
			writeProblem(w, r, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// recoverPanic turns a handler panic into a 500 problem response.
// http.ErrAbortHandler is re-panicked so net/http can abort the connection.
func recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w}
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if v == http.ErrAbortHandler {
				panic(v)
			}
			logger.FromContext(r.Context()).Error("panic recovered", map[string]any{
				"method": r.Method,
				"path":   r.URL.Path,
				"panic":  fmt.Sprint(v),
				"stack":  string(debug.Stack()),
			})
			if !sw.wroteHeader {
				writeProblem(w, r, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(sw, r)
	})
}

// problemFallback replaces ServeMux's plain-text 404 and 405 replies with
// problem responses. The Allow header set for 405 is preserved.
func problemFallback(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := mux.Handler(r); pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}
		mux.ServeHTTP(&fallbackWriter{ResponseWriter: w, r: r}, r)
	})
}

// statusWriter records whether the response header has been sent.
type statusWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(status int) {
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	w.wroteHeader = true
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// fallbackWriter swaps a 404 or 405 reply for a problem body and drops the
// original body. Any other status is passed through untouched.
type fallbackWriter struct {
	http.ResponseWriter
	r           *http.Request
	wroteHeader bool
	replaced    bool
}

func (w *fallbackWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	switch status {
	case http.StatusNotFound:
		w.replaced = true
		w.Header().Del("X-Content-Type-Options")
		writeProblem(w.ResponseWriter, w.r, status, "no route matches "+w.r.URL.Path)
	case http.StatusMethodNotAllowed:
		w.replaced = true
		w.Header().Del("X-Content-Type-Options")
		writeProblem(w.ResponseWriter, w.r, status, "method "+w.r.Method+" is not allowed on "+w.r.URL.Path)
	default:
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *fallbackWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.replaced {
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}

func (w *fallbackWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
