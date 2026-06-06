package ratelimit

import (
	"testing"
	"time"
)

func TestTokenBucketBurstAndRefill(t *testing.T) {
	clock := time.Unix(0, 0)
	l := New(2, 3) // 2/sec, burst 3
	l.now = func() time.Time { return clock }

	// Burst of 3 allowed, then denied.
	for i := 0; i < 3; i++ {
		if !l.Allow("ip") {
			t.Fatalf("burst request %d should be allowed", i)
		}
	}
	if l.Allow("ip") {
		t.Fatal("4th request should be rate-limited")
	}

	// After 1 second, 2 tokens refill.
	clock = clock.Add(time.Second)
	if !l.Allow("ip") || !l.Allow("ip") {
		t.Fatal("two requests should be allowed after 1s refill")
	}
	if l.Allow("ip") {
		t.Fatal("third should be limited again")
	}
}

func TestPerKeyIsolation(t *testing.T) {
	clock := time.Unix(0, 0)
	l := New(1, 1)
	l.now = func() time.Time { return clock }
	if !l.Allow("a") {
		t.Fatal("a first request allowed")
	}
	if l.Allow("a") {
		t.Fatal("a second request limited")
	}
	// Different key has its own bucket.
	if !l.Allow("b") {
		t.Fatal("b should be allowed independently of a")
	}
}

func TestSweepEvictsIdleBuckets(t *testing.T) {
	clock := time.Unix(0, 0)
	l := New(1, 1)
	l.now = func() time.Time { return clock }
	l.Allow("old")
	clock = clock.Add(20 * time.Minute) // idle long enough to be evicted
	l.Allow("new")                      // triggers a sweep
	l.mu.Lock()
	_, oldPresent := l.buckets["old"]
	l.mu.Unlock()
	if oldPresent {
		t.Fatal("idle bucket should have been swept")
	}
}
