// Package storage adapts BoltDB to the todo.Repository port.
package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.etcd.io/bbolt"

	"todo-service/internal/todo"
)

var (
	todosBucket       = []byte("todos")
	idempotencyBucket = []byte("idempotency")
)

// Store is a BoltDB-backed to-do repository.
// A database file can be opened by only one process at a time.
type Store struct {
	db *bbolt.DB
}

// Open opens or creates a BoltDB file at path.
// The caller must Close the store. Open waits up to one second if another
// process already holds the file lock.
func Open(path string) (*Store, error) {
	db, err := bbolt.Open(path, 0o600, &bbolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	err = db.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(todosBucket); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists(idempotencyBucket)
		return err
	})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create bucket: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the database file.
func (s *Store) Close() error {
	return s.db.Close()
}

// Ping reports whether the database is open and the bucket exists.
func (s *Store) Ping(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.db.View(func(tx *bbolt.Tx) error {
		if tx.Bucket(todosBucket) == nil || tx.Bucket(idempotencyBucket) == nil {
			return errors.New("bucket missing")
		}
		return nil
	})
}

type storedTodo struct {
	ID        uint64 `json:"id"`
	PublicID  string `json:"public_id"`
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
}

// List returns every to-do in id order.
func (s *Store) List(ctx context.Context) ([]todo.Todo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	items := []todo.Todo{}
	err := s.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(todosBucket).ForEach(func(_, v []byte) error {
			item, err := decodeTodo(v)
			if err != nil {
				return err
			}
			items = append(items, item)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

// Create inserts item unless idempotencyKey was already stored.
// A repeat returns the original item and does not write again.
func (s *Store) Create(ctx context.Context, item todo.Todo, idempotencyKey string) (todo.Todo, bool, error) {
	if err := ctx.Err(); err != nil {
		return todo.Todo{}, false, err
	}
	if idempotencyKey == "" {
		return todo.Todo{}, false, todo.ErrIdempotencyKey
	}
	var (
		stored  todo.Todo
		created bool
	)
	// Operations are in executed in the same transaction
	err := s.db.Update(func(tx *bbolt.Tx) error {
		seen := tx.Bucket(idempotencyBucket)
		if existing := seen.Get([]byte(idempotencyKey)); existing != nil {
			decoded, err := decodeTodo(existing)
			if err != nil {
				return err
			}
			stored = decoded
			return nil
		}
		todos := tx.Bucket(todosBucket)
		key := []byte(item.PublicID)
		if todos.Get(key) != nil {
			return fmt.Errorf("todo %s already exists", item.PublicID)
		}
		seq, err := todos.NextSequence()
		if err != nil {
			return err
		}
		item.ID = seq
		raw, err := encodeTodo(item)
		if err != nil {
			return err
		}
		if err := todos.Put(key, raw); err != nil {
			return err
		}
		if err := seen.Put([]byte(idempotencyKey), raw); err != nil {
			return err
		}
		stored = item
		created = true
		return nil
	})
	if err != nil {
		return todo.Todo{}, false, err
	}
	return stored, created, nil
}

// Update replaces an existing to-do. It returns todo.ErrNotFound when the id is absent.
// The internal sequence is kept from the stored record.
func (s *Store) Update(ctx context.Context, item todo.Todo) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(todosBucket)
		key := []byte(item.PublicID)
		existing := b.Get(key)
		if existing == nil {
			return todo.ErrNotFound
		}
		current, err := decodeTodo(existing)
		if err != nil {
			return err
		}
		current.Title = item.Title
		current.Completed = item.Completed
		raw, err := encodeTodo(current)
		if err != nil {
			return err
		}
		return b.Put(key, raw)
	})
}

// Delete removes a to-do. It returns todo.ErrNotFound when the id is absent.
func (s *Store) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(todosBucket)
		key := []byte(id) // public id
		if b.Get(key) == nil {
			return todo.ErrNotFound
		}
		return b.Delete(key)
	})
}

func encodeTodo(item todo.Todo) ([]byte, error) {
	return json.Marshal(storedTodo{
		ID:        item.ID,
		PublicID:  item.PublicID,
		Title:     item.Title,
		Completed: item.Completed,
	})
}

func decodeTodo(raw []byte) (todo.Todo, error) {
	var rec storedTodo
	if err := json.Unmarshal(raw, &rec); err != nil {
		return todo.Todo{}, fmt.Errorf("decode todo: %w", err)
	}
	return todo.Todo{ID: rec.ID, PublicID: rec.PublicID, Title: rec.Title, Completed: rec.Completed}, nil
}

var _ todo.Repository = (*Store)(nil)
