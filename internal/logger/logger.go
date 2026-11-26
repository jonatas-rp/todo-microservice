// Package logger writes one JSON object per log line.
// A Logger created for a request is stored in that request's context and is
// never reused for another request.
package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Level is the minimum severity a Logger will write.
type Level int

// LevelDebug is the lowest severity. LevelInfo and LevelError follow it.
const (
	LevelDebug Level = iota
	LevelInfo
	LevelError
)

// ParseLevel reads LOG_LEVEL values. Empty defaults to INFO.
func ParseLevel(s string) (Level, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "", "INFO":
		return LevelInfo, nil
	case "DEBUG":
		return LevelDebug, nil
	case "ERROR":
		return LevelError, nil
	default:
		return 0, fmt.Errorf("LOG_LEVEL must be DEBUG, INFO, or ERROR, got %q", s)
	}
}

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

func severity(name string) Level {
	switch name {
	case "DEBUG":
		return LevelDebug
	case "ERROR":
		return LevelError
	default:
		return LevelInfo
	}
}

// Logger writes structured log lines. Its fields are fixed after New or With,
// so concurrent Info and Error calls do not share mutable request state.
type Logger struct {
	w         io.Writer
	requestID string
	level     Level
	fields    map[string]any
}

type loggerKey struct{}

var (
	levelMu      sync.RWMutex
	defaultLevel = LevelInfo
)

// process is the logger used outside a request. It carries no request id.
var process = New(os.Stdout, "")

// writeMu keeps a single line intact when several loggers share a writer.
var writeMu sync.Mutex

type entry struct {
	Level     string         `json:"level"`
	Timestamp string         `json:"timestamp"`
	Location  string         `json:"location"`
	RequestID string         `json:"request_id,omitempty"`
	Message   string         `json:"message"`
	Context   map[string]any `json:"context,omitempty"`
}

// SetLevel sets the minimum level for New and for the process logger.
func SetLevel(level Level) {
	levelMu.Lock()
	defer levelMu.Unlock()
	defaultLevel = level
	process = &Logger{w: process.w, level: level}
}

func currentLevel() Level {
	levelMu.RLock()
	defer levelMu.RUnlock()
	return defaultLevel
}

// New returns a logger that writes to w. An empty requestID omits the field.
// The logger uses the level set by SetLevel (default INFO).
func New(w io.Writer, requestID string) *Logger {
	if w == nil {
		w = os.Stdout
	}
	return &Logger{w: w, requestID: requestID, level: currentLevel()}
}

// With returns a copy that includes key in its context. The receiver is unchanged.
func (l *Logger) With(key string, value any) *Logger {
	fields := make(map[string]any, len(l.fields)+1)
	for k, v := range l.fields {
		fields[k] = v
	}
	fields[key] = value
	return &Logger{w: l.w, requestID: l.requestID, level: l.level, fields: fields}
}

// WithLevel returns a copy that uses level. The receiver is unchanged.
func (l *Logger) WithLevel(level Level) *Logger {
	return &Logger{w: l.w, requestID: l.requestID, level: level, fields: l.fields}
}

// Debug writes a DEBUG line when the logger level allows it.
func (l *Logger) Debug(message string, context map[string]any) {
	l.emit("DEBUG", message, context)
}

// Info writes an INFO line. context is copied and merged for this line only.
// Pass nil to omit the context field.
func (l *Logger) Info(message string, context map[string]any) {
	l.emit("INFO", message, context)
}

// Error writes an ERROR line. context is copied and merged for this line only.
// Pass nil to omit the context field.
func (l *Logger) Error(message string, context map[string]any) {
	l.emit("ERROR", message, context)
}

func (l *Logger) emit(level, message string, extra map[string]any) {
	if severity(level) < l.level {
		return
	}
	line := entry{
		Level:     level,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Location:  callerLocation(2),
		RequestID: l.requestID,
		Message:   message,
		Context:   l.merged(extra),
	}
	raw, err := json.Marshal(line)
	if err != nil {
		return
	}
	raw = append(raw, '\n')
	writeMu.Lock()
	_, _ = l.w.Write(raw)
	writeMu.Unlock()
}

// merged copies the logger fields and extra so neither map is retained or mutated.
func (l *Logger) merged(extra map[string]any) map[string]any {
	if len(l.fields) == 0 && len(extra) == 0 {
		return nil
	}
	out := make(map[string]any, len(l.fields)+len(extra))
	for k, v := range l.fields {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func callerLocation(skip int) string {
	_, file, line, ok := runtime.Caller(skip + 1)
	if !ok {
		return "unknown"
	}
	const marker = "todo-service/"
	if i := strings.LastIndex(file, marker); i >= 0 {
		file = file[i+len(marker):]
	} else {
		file = filepath.Base(file)
	}
	return file + ":" + strconv.Itoa(line)
}

// NewContext returns a context that carries l for this request only.
func NewContext(ctx context.Context, l *Logger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, loggerKey{}, l)
}

// FromContext returns the request logger, or a process logger with no request id.
func FromContext(ctx context.Context) *Logger {
	if ctx != nil {
		if l, ok := ctx.Value(loggerKey{}).(*Logger); ok && l != nil {
			return l
		}
	}
	return process
}

// WithRequestID returns a copy that writes requestID. The receiver is unchanged.
func (l *Logger) WithRequestID(requestID string) *Logger {
	if l == nil {
		return New(os.Stdout, requestID)
	}
	return &Logger{w: l.w, requestID: requestID, level: l.level, fields: l.fields}
}
