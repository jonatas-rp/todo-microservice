// Package main opens the BoltDB file and serves the store API over gRPC.
package main

import (
	"context"
	"errors"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"todo-service/internal/config"
	"todo-service/internal/logger"
	"todo-service/internal/storage"
	"todo-service/internal/store/pb"
	"todo-service/internal/store/server"
)

func main() {
	log := logger.New(os.Stdout, "")
	if err := run(log); err != nil {
		log.Error("store exited", map[string]any{"err": err.Error()})
		os.Exit(1)
	}
}

func run(log *logger.Logger) (err error) {
	cfg, err := config.LoadStore()
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
	defer func() {
		err = errors.Join(err, store.Close())
	}()

	lis, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return err
	}
	gs := grpc.NewServer()
	pb.RegisterStoreServer(gs, server.New(store, log))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", map[string]any{"addr": cfg.Addr, "db": cfg.DBPath, "log_level": cfg.LogLevel})
		errCh <- gs.Serve(lis)
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, grpc.ErrServerStopped) {
			return nil
		}
		return err
	case <-ctx.Done():
		log.Info("shutting down", nil)
		stopped := make(chan struct{})
		go func() {
			gs.GracefulStop()
			close(stopped)
		}()
		select {
		case <-stopped:
			return nil
		case <-time.After(10 * time.Second):
			gs.Stop()
			return nil
		}
	}
}
