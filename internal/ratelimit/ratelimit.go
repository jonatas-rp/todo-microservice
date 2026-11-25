// Package ratelimit limits requests with a token bucket.
package ratelimit

import "time"

// Limiter allows a burst of capacity requests, then refills perSecond tokens.
type Limiter struct {
	store     Store
	capacity  float64
	perSecond float64
}

// New returns a limiter that stores bucket state in store.
func New(store Store, capacity, perSecond float64) *Limiter {
	return &Limiter{store: store, capacity: capacity, perSecond: perSecond}
}

// Allow reports whether key may proceed at now. One allowed call spends one token.
func (l *Limiter) Allow(key string, now time.Time) bool {
	allowed := false
	l.store.Update(key, func(state State) State {
		tokens := l.capacity
		if !state.LastSeen.IsZero() {
			tokens = state.Tokens + now.Sub(state.LastSeen).Seconds()*l.perSecond
			if tokens > l.capacity {
				tokens = l.capacity
			}
		}
		if tokens >= 1 {
			tokens--
			allowed = true
		}
		return State{Tokens: tokens, LastSeen: now}
	})
	return allowed
}
