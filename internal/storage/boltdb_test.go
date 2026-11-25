package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"todo-service/internal/todo"
)

func TestStoreCRUD(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	ctx := context.Background()

	if err := store.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	items, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("List = %+v", items)
	}

	item := todo.Todo{PublicID: "pub-1", Title: "buy milk", Completed: false}
	stored, created, err := store.Create(ctx, item, "key-1")
	if err != nil || !created || stored.ID == 0 || stored.PublicID != item.PublicID || stored.Title != item.Title {
		t.Fatalf("Create = %+v %v %v", stored, created, err)
	}
	again, created, err := store.Create(ctx, todo.Todo{PublicID: "pub-2", Title: "other", Completed: true}, "key-1")
	if err != nil || created || again != stored {
		t.Fatalf("replay = %+v %v %v", again, created, err)
	}
	if _, _, err := store.Create(ctx, item, "key-2"); err == nil {
		t.Fatal("Create duplicate id: expected error")
	}

	stored.Title = "buy oats"
	stored.Completed = true
	if err := store.Update(ctx, stored); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := store.Update(ctx, todo.Todo{PublicID: "missing", Title: "x"}); !errors.Is(err, todo.ErrNotFound) {
		t.Fatalf("Update missing: %v", err)
	}

	items, err = store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0] != stored {
		t.Fatalf("List = %+v, want %+v", items, stored)
	}

	if err := store.Delete(ctx, stored.PublicID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := store.Delete(ctx, stored.PublicID); !errors.Is(err, todo.ErrNotFound) {
		t.Fatalf("Delete missing: %v", err)
	}
	items, err = store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("List after delete = %+v", items)
	}
}

func TestStorePersistsAcrossReopen(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "todos.db")
	ctx := context.Background()
	item := todo.Todo{PublicID: "pub-1", Title: "buy milk", Completed: true}

	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	stored, _, err := store.Create(ctx, item, "key-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	store, err = Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	items, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0] != stored {
		t.Fatalf("List = %+v, want %+v", items, stored)
	}
}

func TestStoreEmptyIdempotencyKey(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	_, _, err := store.Create(context.Background(), todo.Todo{PublicID: "pub-1", Title: "x"}, "")
	if !errors.Is(err, todo.ErrIdempotencyKey) {
		t.Fatalf("Create empty key: %v", err)
	}
}

func TestStorePingAfterClose(t *testing.T) {
	t.Parallel()
	store, err := Open(filepath.Join(t.TempDir(), "todos.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := store.Ping(context.Background()); err == nil {
		t.Fatal("Ping after Close: expected error")
	}
}

func TestStoreCanceledContext(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := store.List(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("List: %v", err)
	}
	if _, _, err := store.Create(ctx, todo.Todo{PublicID: "pub-1", Title: "x"}, "key"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Update(ctx, todo.Todo{PublicID: "pub-1", Title: "x"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Update: %v", err)
	}
	if err := store.Delete(ctx, "pub-1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Delete: %v", err)
	}
	if err := store.Ping(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Ping: %v", err)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "todos.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
