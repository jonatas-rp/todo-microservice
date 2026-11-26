package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("DB_PATH", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("STORE_ADDR", "")
	t.Setenv("PATH_PREFIX", "")
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
	if cfg.Addr != ":1" || cfg.StoreAddr != "" || cfg.DBPath != "todos.db" {
		t.Fatalf("PORT 1 = %+v", cfg)
	}

	t.Setenv("STORE_ADDR", "todo-store:50051")
	t.Setenv("DB_PATH", "/ignored/todos.db")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.StoreAddr != "todo-store:50051" || cfg.DBPath != "" {
		t.Fatalf("STORE_ADDR ignores DB_PATH, got %+v", cfg)
	}

	t.Setenv("PATH_PREFIX", "/dev")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PathPrefix != "/dev" {
		t.Fatalf("PATH_PREFIX = %q", cfg.PathPrefix)
	}
	for _, raw := range []string{"dev", "/dev/", "/Dev", "/dev/v1"} {
		t.Setenv("PATH_PREFIX", raw)
		if _, err := Load(); err == nil {
			t.Fatalf("expected invalid PATH_PREFIX %q", raw)
		}
	}
}

func TestLoadStore(t *testing.T) {
	t.Setenv("GRPC_ADDR", "")
	t.Setenv("DB_PATH", "")
	t.Setenv("LOG_LEVEL", "")
	cfg, err := LoadStore()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":50051" || cfg.DBPath != "todos.db" || cfg.LogLevel != "INFO" {
		t.Fatalf("defaults = %+v", cfg)
	}

	t.Setenv("GRPC_ADDR", "50051")
	t.Setenv("DB_PATH", "/data/todos.db")
	t.Setenv("LOG_LEVEL", "error")
	cfg, err = LoadStore()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":50051" || cfg.DBPath != "/data/todos.db" || cfg.LogLevel != "ERROR" {
		t.Fatalf("port number = %+v", cfg)
	}

	t.Setenv("GRPC_ADDR", "0.0.0.0:50051")
	cfg, err = LoadStore()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "0.0.0.0:50051" {
		t.Fatalf("host address = %+v", cfg)
	}

	t.Setenv("GRPC_ADDR", "nope")
	if _, err := LoadStore(); err == nil {
		t.Fatal("expected invalid GRPC_ADDR error")
	}
	t.Setenv("GRPC_ADDR", ":50051")
	t.Setenv("LOG_LEVEL", "nope")
	if _, err := LoadStore(); err == nil {
		t.Fatal("expected invalid LOG_LEVEL error")
	}
}
