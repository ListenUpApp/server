// Package ratelimit provides a keyed rate limiter using token bucket algorithm.
// It supports both non-blocking (Allow) and blocking (Wait) operations.
package ratelimit

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
)

// KeyedRateLimiter manages per-key rate limiting.
// Each unique key gets its own independent rate limiter.
type KeyedRateLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*rate.Limiter
	limit    rate.Limit
	burst    int

	// Cleanup
	done     chan struct{}
	stopOnce sync.Once
}

// New creates a new keyed rate limiter.
// rps: requests per second allowed.
// burst: maximum burst size (tokens available immediately).
func New(rps float64, burst int) *KeyedRateLimiter {
	krl := &KeyedRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		limit:    rate.Limit(rps),
		burst:    burst,
		done:     make(chan struct{}),
	}

	go krl.cleanup()

	return krl
}

// Allow checks if a request for the given key should be allowed.
// Returns immediately without blocking. Use for inbound request protection.
func (krl *KeyedRateLimiter) Allow(key string) bool {
	return krl.getLimiter(key).Allow()
}

// Wait blocks until a request for the given key is allowed or context is canceled.
// Use for outbound requests where you want to respect rate limits.
func (krl *KeyedRateLimiter) Wait(ctx context.Context, key string) error {
	return krl.getLimiter(key).Wait(ctx)
}

// getLimiter returns the limiter for a key, creating one if needed.
func (krl *KeyedRateLimiter) getLimiter(key string) *rate.Limiter {
	// Fast path: read lock
	krl.mu.RLock()
	limiter, exists := krl.limiters[key]
	krl.mu.RUnlock()

	if exists {
		return limiter
	}

	// Slow path: write lock to create
	krl.mu.Lock()
	defer krl.mu.Unlock()

	// Double-check after acquiring write lock
	if limiter, exists = krl.limiters[key]; exists {
		return limiter
	}

	limiter = rate.NewLimiter(krl.limit, krl.burst)
	krl.limiters[key] = limiter
	return limiter
}

// Stop shuts down the cleanup goroutine.
func (krl *KeyedRateLimiter) Stop() {
	krl.stopOnce.Do(func() {
		close(krl.done)
	})
}

// cleanup waits for the stop signal.
// Note: Currently no cleanup is performed since rate.Limiter doesn't track
// last access time. For Audible (10 regions) this is not a concern.
// In production with many keys, you might wrap the limiter with last-access tracking.
func (krl *KeyedRateLimiter) cleanup() {
	<-krl.done
}
