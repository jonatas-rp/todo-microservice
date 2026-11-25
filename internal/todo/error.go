package todo

import "errors"

var (
	// ErrNotFound is returned when no to-do exists for an id.
	ErrNotFound = errors.New("todo not found")

	// ErrEmptyTitle is returned when a title is missing or blank.
	ErrEmptyTitle = errors.New("title is required")

	// ErrIdempotencyKey is returned when a create key is missing or not a UUID.
	ErrIdempotencyKey = errors.New("idempotency key must be a UUID")
)
