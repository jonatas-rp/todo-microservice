package ratelimit_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"todo-service/internal/ratelimit"
	"todo-service/internal/storage"
)

func TestAllowBurstAndRefill(t *testing.T) {
	lim := ratelimit.New(storage.NewMemory(), 2, 1)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if !lim.Allow("203.0.113.7", now) {
		t.Fatal("burst of 2 was denied")
	}
	if lim.Allow("203.0.113.7", now) {
		t.Fatal("third request was allowed")
	}
	if !lim.Allow("198.51.100.4", now) {
		t.Fatal("a different client was limited")
	}
	later := now.Add(time.Second)
	if !lim.Allow("203.0.113.7", later) {
		t.Fatal("refilled token was denied")
	}
	if lim.Allow("203.0.113.7", later) {
		t.Fatal("request beyond the refill was allowed")
	}
}

func TestAllowRefillCapsAtCapacity(t *testing.T) {
	lim := ratelimit.New(storage.NewMemory(), 2, 100)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if !lim.Allow("203.0.113.7", now) {
		t.Fatal("first request denied")
	}
	later := now.Add(time.Hour)
	if !lim.Allow("203.0.113.7", later) {
		t.Fatal("refill did not restore capacity")
	}
	if lim.Allow("203.0.113.7", later) {
		t.Fatal("allowed more than capacity after a long wait")
	}
}

func TestAllowParallelBurst(t *testing.T) {
	const capacity = 20
	lim := ratelimit.New(storage.NewMemory(), capacity, 0)
	now := time.Now()

	var wg sync.WaitGroup
	var allowed atomic.Int32
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if lim.Allow("203.0.113.7", now) {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := allowed.Load(); got != capacity {
		t.Fatalf("allowed = %d, want %d", got, capacity)
	}
}
