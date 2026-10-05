package main

import (
	"fmt"
	"sync"
	"time"
)

type rateLimiter struct {
	mu       sync.Mutex
	limits   map[string]*bucket
	interval time.Duration
	burst    int
}

type bucket struct {
	tokens    int
	lastReset time.Time
}

func newRateLimiter(interval time.Duration, burst int) *rateLimiter {
	return &rateLimiter{
		limits:   make(map[string]*bucket),
		interval: interval,
		burst:    burst,
	}
}

// grant adds n extra tokens to key's current window, for work whose size
// the backend itself knows to be bounded (restoring a saved session).
func (r *rateLimiter) grant(key string, n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.limits[key]
	if !ok {
		b = &bucket{tokens: r.burst, lastReset: time.Now()}
		r.limits[key] = b
	}
	b.tokens += n
}

func (r *rateLimiter) allow(key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	b, ok := r.limits[key]
	if !ok {
		b = &bucket{tokens: r.burst, lastReset: time.Now()}
		r.limits[key] = b
	}

	now := time.Now()
	elapsed := now.Sub(b.lastReset)
	if elapsed >= r.interval {
		b.tokens = r.burst
		b.lastReset = now
	}

	if b.tokens <= 0 {
		return fmt.Errorf("rate limit exceeded for %s, try again shortly", key)
	}
	b.tokens--
	return nil
}
