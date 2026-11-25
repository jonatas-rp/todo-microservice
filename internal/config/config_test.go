package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("DB_PATH", "")
	t.Setenv("LOG_LEVEL", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":8080" || cfg.DBPath != "todos.db" || cfg.LogLevel != "INFO" {
		t.Fatalf("defaults = %+v", cfg)
	}

	t.Setenv("PORT", "09090")
	t.Setenv("DB_PATH", "/tmp/todos.db")
	t.Setenv("LOG_LEVEL", "debug")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":9090" || cfg.DBPath != "/tmp/todos.db" || cfg.LogLevel != "DEBUG" {
		t.Fatalf("override = %+v", cfg)
	}

	t.Setenv("LOG_LEVEL", "nope")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid LOG_LEVEL error")
	}
	t.Setenv("LOG_LEVEL", "INFO")

	t.Setenv("PORT", "nope")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid PORT error")
	}

	t.Setenv("PORT", "0")
	if _, err := Load(); err == nil {
		t.Fatal("expected PORT 0 error")
	}
	t.Setenv("PORT", "65536")
	if _, err := Load(); err == nil {
		t.Fatal("expected PORT 65536 error")
	}

	t.Setenv("PORT", "1")
	t.Setenv("DB_PATH", "")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":1" {
		t.Fatalf("PORT 1 = %+v", cfg)
	}
}
