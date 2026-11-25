package todo

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// Service implements the to-do use cases.
type Service struct {
	repo Repository
}

// New returns a service that stores items through repo.
func New(repo Repository) *Service {
	return &Service{repo: repo}
}

// List returns every to-do. The slice is empty when none exist.
func (s *Service) List(ctx context.Context) ([]Todo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	if items == nil {
		return []Todo{}, nil
	}
	return items, nil
}

// Create stores a new to-do and returns it with a generated id.
// created is false when idempotencyKey was already used; the returned item is the original.
func (s *Service) Create(ctx context.Context, title string, completed bool, idempotencyKey string) (Todo, bool, error) {
	if err := ctx.Err(); err != nil {
		return Todo{}, false, err
	}
	id, err := uuid.Parse(strings.TrimSpace(idempotencyKey))
	if err != nil {
		return Todo{}, false, ErrIdempotencyKey
	}
	item, err := newTodo(title, completed)
	if err != nil {
		return Todo{}, false, err
	}
	return s.repo.Create(ctx, item, id.String())
}

// Update replaces the title and completed flag of an existing to-do.
func (s *Service) Update(ctx context.Context, id, title string, completed bool) (Todo, error) {
	if err := ctx.Err(); err != nil {
		return Todo{}, err
	}
	if id == "" {
		return Todo{}, ErrNotFound
	}
	title, err := validTitle(title)
	if err != nil {
		return Todo{}, err
	}
	item := Todo{PublicID: id, Title: title, Completed: completed}
	if err := s.repo.Update(ctx, item); err != nil {
		return Todo{}, err
	}
	return item, nil
}

// Delete removes a to-do by id.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if id == "" {
		return ErrNotFound
	}
	return s.repo.Delete(ctx, id)
}
