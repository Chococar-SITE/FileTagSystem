// Package ratelimit provides a simple in-memory token-bucket limiter keyed by
// an arbitrary string (e.g. client IP), used for the general per-client API
// throttle (§7.4). It is suitable for a single instance; a shared store would be
// needed across horizontally-scaled instances.
package ratelimit

import (
	"sync"
	"time"
)

type bucket struct {
	tokens float64
	last   time.Time
}

// Limiter is a token-bucket rate limiter.
type Limiter struct {
	mu        sync.Mutex
	buckets   map[string]*bucket
	rate      float64 // tokens added per second
	burst     float64 // bucket capacity
	lastSweep time.Time
	now       func() time.Time // injectable for tests
}

// New returns a limiter allowing `rate` requests/second per key with a burst
// capacity of `burst`.
func New(rate, burst float64) *Limiter {
	if rate <= 0 {
		rate = 1
	}
	if burst < 1 {
		burst = 1
	}
	return &Limiter{
		buckets: make(map[string]*bucket),
		rate:    rate,
		burst:   burst,
		now:     time.Now,
	}
}

// Allow reports whether a request for key may proceed, consuming one token.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	} else {
		elapsed := now.Sub(b.last).Seconds()
		if elapsed > 0 {
			b.tokens = min(l.burst, b.tokens+elapsed*l.rate)
			b.last = now
		}
	}
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// sweep periodically drops idle buckets so the map can't grow unbounded with
// unique keys (e.g. spoofed/changing IPs). Cheap: at most once per minute.
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < time.Minute {
		return
	}
	l.lastSweep = now
	for k, b := range l.buckets {
		if now.Sub(b.last) > 10*time.Minute {
			delete(l.buckets, k)
		}
	}
}
