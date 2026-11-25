package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"todo-service/internal/config"
	"todo-service/internal/httpapi"
	"todo-service/internal/logger"
	"todo-service/internal/ratelimit"
	"todo-service/internal/storage"
	"todo-service/internal/todo"
)

func main() {
	log := logger.New(os.Stdout, "")
	if err := run(log); err != nil {
		log.Error("server exited", map[string]any{"err": err.Error()})
		os.Exit(1)
	}
}

func run(log *logger.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	level, err := logger.ParseLevel(cfg.LogLevel)
	if err != nil {
		return err
	}
	logger.SetLevel(level)
	log = log.WithLevel(level)

	store, err := storage.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()

	svc := todo.New(store)
	lim := ratelimit.New(storage.NewMemory(), 10, 1)
	readiness := httpapi.NewReadiness()
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           httpapi.NewHandler(svc, store, lim, readiness),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Handle SIGTERM for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", map[string]any{"addr": cfg.Addr, "db": cfg.DBPath, "log_level": cfg.LogLevel})
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		log.Info("shutting down", nil)
		readiness.SetReadiness(false)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
