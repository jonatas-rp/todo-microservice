package main

import (
	"path/filepath"
	"testing"

	"todo-service/internal/logger"
)

func TestRunBadDBPath(t *testing.T) {
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "missing", "todos.db"))
	if err := run(logger.New(t.Output(), "")); err == nil {
		t.Fatal("expected open database error")
	}
}
