package storage

import (
	"sync"

	"todo-service/internal/ratelimit"
)

// Memory is an in-memory ratelimit.Store keyed by client id.
// The mutex covers the map and the Update callback.
type Memory struct {
	mu      sync.Mutex
	buckets map[string]ratelimit.State
}

// NewMemory returns an empty rate-limit store.
func NewMemory() *Memory {
	return &Memory{buckets: map[string]ratelimit.State{}}
}

// Update applies fn to the state for key and stores the result.
func (m *Memory) Update(key string, fn func(ratelimit.State) ratelimit.State) ratelimit.State {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := fn(m.buckets[key])
	m.buckets[key] = next
	return next
}

var _ ratelimit.Store = (*Memory)(nil)
