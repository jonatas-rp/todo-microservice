package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc"

	"todo-service/internal/logger"
	"todo-service/internal/storage"
	"todo-service/internal/store/client"
	"todo-service/internal/store/pb"
	"todo-service/internal/store/server"
	"todo-service/internal/todo"
)

func TestStoreGRPC(t *testing.T) {
	c := dial(t)

	ctx := context.Background()
	if err := c.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	items, err := c.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("List = %+v, want empty", items)
	}

	item := todo.Todo{PublicID: "pub-1", Title: "buy milk", Completed: false}
	stored, created, err := c.Create(ctx, item, "key-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !created || stored.PublicID != item.PublicID || stored.Title != item.Title || stored.ID == 0 {
		t.Fatalf("Create = %+v created=%v", stored, created)
	}

	again, created, err := c.Create(ctx, todo.Todo{PublicID: "pub-2", Title: "other", Completed: true}, "key-1")
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if created || again.PublicID != stored.PublicID || again.Title != stored.Title {
		t.Fatalf("replay = %+v created=%v, want original %+v", again, created, stored)
	}

	stored.Title = "buy oats"
	stored.Completed = true
	if err := c.Update(ctx, stored); err != nil {
		t.Fatalf("Update: %v", err)
	}
	items, err = c.List(ctx)
	if err != nil {
		t.Fatalf("List after update: %v", err)
	}
	if len(items) != 1 || items[0].Title != "buy oats" || !items[0].Completed || items[0].PublicID != "pub-1" {
		t.Fatalf("List = %+v", items)
	}

	if err := c.Update(ctx, todo.Todo{PublicID: "missing", Title: "x"}); !errors.Is(err, todo.ErrNotFound) {
		t.Fatalf("Update missing = %v, want ErrNotFound", err)
	}
	if err := c.Delete(ctx, "missing"); !errors.Is(err, todo.ErrNotFound) {
		t.Fatalf("Delete missing = %v, want ErrNotFound", err)
	}
	if err := c.Delete(ctx, "pub-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	items, err = c.List(ctx)
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("List = %+v, want empty", items)
	}
}

func TestRequestIDReachesStoreLog(t *testing.T) {
	var buf bytes.Buffer
	store, err := storage.Open(filepath.Join(t.TempDir(), "todos.db"))
	if err != nil {
		t.Fatal(err)
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gs := grpc.NewServer()
	pb.RegisterStoreServer(gs, server.New(store, logger.New(&buf, "")))
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(func() {
		gs.Stop()
		_ = store.Close()
	})

	c, err := client.Dial(context.Background(), lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })

	ctx := context.WithValue(context.Background(), "request_id", "req-1")
	if _, err := c.List(ctx); err != nil {
		t.Fatal(err)
	}

	var line struct {
		RequestID string `json:"request_id"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line); err != nil {
		t.Fatal(err)
	}
	if line.RequestID != "req-1" || line.Message != "list" {
		t.Fatalf("log = %+v, body %s", line, buf.String())
	}
}

func dial(t *testing.T) *client.Client {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "todos.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	gs := grpc.NewServer()
	pb.RegisterStoreServer(gs, server.New(store, logger.New(os.Stdout, "")))
	go func() { _ = gs.Serve(lis) }()

	c, err := client.Dial(context.Background(), lis.Addr().String())
	if err != nil {
		gs.Stop()
		_ = store.Close()
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() {
		_ = c.Close()
		gs.Stop()
		_ = store.Close()
	})
	return c
}
