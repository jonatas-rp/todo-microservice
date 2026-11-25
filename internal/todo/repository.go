package todo

import "context"

// Repository is the persistence port for to-do items.
// Storage adapters implement it; this package does not import them.
type Repository interface {
	List(ctx context.Context) ([]Todo, error)
	// Create stores item under idempotencyKey. A repeated key returns the
	// original item and created=false without writing again.
	Create(ctx context.Context, item Todo, idempotencyKey string) (stored Todo, created bool, err error)
	Update(ctx context.Context, item Todo) error
	Delete(ctx context.Context, id string) error
}
