package ratelimit

import "time"

// State is the token bucket stored for one client.
type State struct {
	Tokens   float64
	LastSeen time.Time
}

// Store persists bucket state. Update calls fn synchronously while key is locked.
// A missing key is passed as the zero State.
type Store interface {
	Update(key string, fn func(State) State) State
}
