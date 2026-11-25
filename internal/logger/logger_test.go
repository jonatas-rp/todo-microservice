package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLineSchema(t *testing.T) {
	var buf bytes.Buffer
	New(&buf, "req-1").Info("hello", map[string]any{"path": "/todos"})

	raw := strings.TrimSpace(buf.String())
	keys := []string{`"level"`, `"timestamp"`, `"location"`, `"request_id"`, `"message"`, `"context"`}
	last := -1
	for _, key := range keys {
		i := strings.Index(raw, key)
		if i <= last {
			t.Fatalf("key %s out of order in %s", key, raw)
		}
		last = i
	}

	var line entry
	if err := json.Unmarshal([]byte(raw), &line); err != nil {
		t.Fatal(err)
	}
	if line.Level != "INFO" || line.RequestID != "req-1" || line.Message != "hello" {
		t.Fatalf("line = %+v", line)
	}
	if line.Context["path"] != "/todos" {
		t.Fatalf("context = %+v", line.Context)
	}
	if _, err := time.Parse(time.RFC3339Nano, line.Timestamp); err != nil {
		t.Fatalf("timestamp %q: %v", line.Timestamp, err)
	}
	if !strings.Contains(line.Location, "logger_test.go:") {
		t.Fatalf("location = %q", line.Location)
	}
}

func TestContextOmittedWhenEmpty(t *testing.T) {
	var buf bytes.Buffer
	New(&buf, "").Info("listening", nil)

	raw := buf.String()
	if strings.Contains(raw, `"request_id"`) || strings.Contains(raw, `"context"`) {
		t.Fatalf("empty fields leaked: %s", raw)
	}
}

func TestWithDoesNotChangeParent(t *testing.T) {
	var buf bytes.Buffer
	parent := New(&buf, "req-1")
	child := parent.With("component", "http")
	parent.Info("parent", nil)
	child.Info("child", map[string]any{"path": "/todos"})
	parent.Info("parent again", nil)

	lines := decodeLines(t, &buf)
	if len(lines) != 3 {
		t.Fatalf("lines = %d", len(lines))
	}
	if lines[0].Context != nil || lines[2].Context != nil {
		t.Fatalf("parent context = %+v / %+v", lines[0].Context, lines[2].Context)
	}
	if lines[1].Context["component"] != "http" || lines[1].Context["path"] != "/todos" {
		t.Fatalf("child context = %+v", lines[1].Context)
	}
	if lines[0].RequestID != "req-1" || lines[1].RequestID != "req-1" {
		t.Fatalf("request ids = %q %q", lines[0].RequestID, lines[1].RequestID)
	}
}

func TestConcurrentLoggersKeepTheirRequestID(t *testing.T) {
	var buf bytes.Buffer
	var wg sync.WaitGroup
	for _, id := range []string{"req-a", "req-b"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			l := New(&buf, id)
			for range 40 {
				l.Info("tick", map[string]any{"id": id})
			}
		}(id)
	}
	wg.Wait()

	for _, line := range decodeLines(t, &buf) {
		if line.RequestID != line.Context["id"] {
			t.Fatalf("mixed line %+v", line)
		}
	}
}

func TestLevelFiltersLines(t *testing.T) {
	t.Cleanup(func() { SetLevel(LevelInfo) })

	var buf bytes.Buffer
	errorOnly := New(&buf, "").WithLevel(LevelError)
	errorOnly.Debug("d", nil)
	errorOnly.Info("i", nil)
	errorOnly.Error("e", nil)
	if got := strings.TrimSpace(buf.String()); !strings.Contains(got, `"level":"ERROR"`) || strings.Contains(got, `"level":"INFO"`) || strings.Contains(got, `"level":"DEBUG"`) {
		t.Fatalf("error level = %s", got)
	}

	buf.Reset()
	SetLevel(LevelDebug)
	debug := New(&buf, "")
	debug.Debug("d", nil)
	debug.Info("i", nil)
	if !strings.Contains(buf.String(), `"level":"DEBUG"`) || !strings.Contains(buf.String(), `"level":"INFO"`) {
		t.Fatalf("debug level = %s", buf.String())
	}
}

func TestParseLevel(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Level
	}{
		{"", LevelInfo},
		{"info", LevelInfo},
		{"DEBUG", LevelDebug},
		{"Error", LevelError},
	} {
		got, err := ParseLevel(tc.in)
		if err != nil || got != tc.want {
			t.Fatalf("ParseLevel(%q) = %v %v", tc.in, got, err)
		}
	}
	if _, err := ParseLevel("trace"); err == nil {
		t.Fatal("expected error")
	}
}

func TestFromContextIsRequestLocal(t *testing.T) {
	a := New(&bytes.Buffer{}, "req-a")
	b := New(&bytes.Buffer{}, "req-b")
	ctxA := NewContext(context.Background(), a)
	ctxB := NewContext(context.Background(), b)

	if FromContext(ctxA) != a || FromContext(ctxB) != b {
		t.Fatal("context returned the wrong logger")
	}
	if got := FromContext(context.Background()); got.requestID != "" || got == a || got == b {
		t.Fatal("background context leaked a request logger")
	}
}

func decodeLines(t *testing.T, buf *bytes.Buffer) []entry {
	t.Helper()
	var lines []entry
	for _, raw := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var line entry
		if err := json.Unmarshal([]byte(raw), &line); err != nil {
			t.Fatalf("decode %s: %v", raw, err)
		}
		lines = append(lines, line)
	}
	return lines
}
