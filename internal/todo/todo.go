// Package todo is the application core: the to-do model, the repository port,
// and the use cases. Adapters (HTTP, BoltDB) depend on this package; it does
// not depend on them.
package todo

import (
	"strings"

	"github.com/google/uuid"
)

// Todo is one to-do item.
// ID is the internal sequence and is not part of the JSON response.
// PublicID is the client-facing identifier.
type Todo struct {
	ID        uint64 `json:"-"`
	PublicID  string `json:"id"`
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
}

// newTodo validates the title and returns a to-do with a new public id.
// The internal ID stays zero until storage assigns it.
func newTodo(title string, completed bool) (Todo, error) {
	title, err := validTitle(title)
	if err != nil {
		return Todo{}, err
	}
	return Todo{PublicID: uuid.NewString(), Title: title, Completed: completed}, nil
}

// validTitle trims title and rejects it when blank.
func validTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", ErrEmptyTitle
	}
	return title, nil
}
