package auth

import (
	"sync"
	"time"
)

// Limiter is a simple in-memory failure counter with temporary lockout, used
// for per-IP and per-account login throttling (§7.4). Suitable for a single
// instance; a shared store would be needed for horizontal scaling.
type Limiter struct {
	mu      sync.Mutex
	fails   map[string]int
	until   map[string]time.Time
	max     int
	lockout time.Duration
}

// NewLimiter builds a limiter that locks a key for lockout after max failures.
func NewLimiter(max int, lockout time.Duration) *Limiter {
	return &Limiter{
		fails:   make(map[string]int),
		until:   make(map[string]time.Time),
		max:     max,
		lockout: lockout,
	}
}

// Allowed reports whether key is currently permitted to attempt.
func (l *Limiter) Allowed(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if t, ok := l.until[key]; ok {
		if time.Now().Before(t) {
			return false
		}
		delete(l.until, key)
		delete(l.fails, key)
	}
	return true
}

// Fail records a failure, locking the key once the threshold is reached.
func (l *Limiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fails[key]++
	if l.max > 0 && l.fails[key] >= l.max {
		l.until[key] = time.Now().Add(l.lockout)
	}
}

// Reset clears a key's failure state (after a successful attempt).
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, key)
	delete(l.until, key)
}
