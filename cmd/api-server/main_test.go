package main

import (
	"path/filepath"
	"testing"

	"todo-service/internal/logger"
)

func TestRunInvalidPORT(t *testing.T) {
	t.Setenv("PORT", "nope")
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "todos.db"))
	if err := run(logger.New(t.Output(), "")); err == nil {
		t.Fatal("expected invalid PORT error")
	}
}

func TestRunBadDBPath(t *testing.T) {
	t.Setenv("PORT", "18080")
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "missing", "todos.db"))
	if err := run(logger.New(t.Output(), "")); err == nil {
		t.Fatal("expected open database error")
	}
}
