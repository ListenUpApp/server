// Package ratelimit provides a keyed rate limiter using token bucket algorithm.
// It supports both non-blocking (Allow) and blocking (Wait) operations.
package ratelimit

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// limiterEntry wraps a rate.Limiter with last-access tracking for TTL cleanup.
type limiterEntry struct {
	limiter    *rate.Limiter
	lastAccess time.Time
}

// KeyedRateLimiter manages per-key rate limiting.
// Each unique key gets its own independent rate limiter.
type KeyedRateLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*limiterEntry
	limit    rate.Limit
	burst    int

	// TTL for inactive limiters (default: 10 minutes)
	ttl time.Duration

	// Cleanup interval (default: 1 minute)
	cleanupInterval time.Duration

	// Cleanup
	done     chan struct{}
	stopOnce sync.Once
}

// Option configures the KeyedRateLimiter.
type Option func(*KeyedRateLimiter)

// WithTTL sets the TTL for inactive limiters.
func WithTTL(ttl time.Duration) Option {
	return func(krl *KeyedRateLimiter) {
		krl.ttl = ttl
	}
}

// WithCleanupInterval sets how often to run cleanup.
func WithCleanupInterval(interval time.Duration) Option {
	return func(krl *KeyedRateLimiter) {
		krl.cleanupInterval = interval
	}
}

// New creates a new keyed rate limiter.
// rps: requests per second allowed.
// burst: maximum burst size (tokens available immediately).
func New(rps float64, burst int, opts ...Option) *KeyedRateLimiter {
	krl := &KeyedRateLimiter{
		limiters:        make(map[string]*limiterEntry),
		limit:           rate.Limit(rps),
		burst:           burst,
		ttl:             10 * time.Minute, // Default TTL
		cleanupInterval: 1 * time.Minute,  // Default cleanup interval
		done:            make(chan struct{}),
	}

	for _, opt := range opts {
		opt(krl)
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
// Updates last-access time on each access.
func (krl *KeyedRateLimiter) getLimiter(key string) *rate.Limiter {
	now := time.Now()

	// Fast path: read lock
	krl.mu.RLock()
	entry, exists := krl.limiters[key]
	krl.mu.RUnlock()

	if exists {
		// Update last access time (requires write lock)
		krl.mu.Lock()
		if entry, ok := krl.limiters[key]; ok {
			entry.lastAccess = now
		}
		krl.mu.Unlock()
		return entry.limiter
	}

	// Slow path: write lock to create
	krl.mu.Lock()
	defer krl.mu.Unlock()

	// Double-check after acquiring write lock
	if entry, exists = krl.limiters[key]; exists {
		entry.lastAccess = now
		return entry.limiter
	}

	limiter := rate.NewLimiter(krl.limit, krl.burst)
	krl.limiters[key] = &limiterEntry{
		limiter:    limiter,
		lastAccess: now,
	}
	return limiter
}

// Stop shuts down the cleanup goroutine.
func (krl *KeyedRateLimiter) Stop() {
	krl.stopOnce.Do(func() {
		close(krl.done)
	})
}

// cleanup periodically removes stale limiters that haven't been accessed within TTL.
func (krl *KeyedRateLimiter) cleanup() {
	ticker := time.NewTicker(krl.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-krl.done:
			return
		case <-ticker.C:
			krl.removeStale()
		}
	}
}

// removeStale removes limiters that haven't been accessed within the TTL.
func (krl *KeyedRateLimiter) removeStale() {
	now := time.Now()
	threshold := now.Add(-krl.ttl)

	krl.mu.Lock()
	defer krl.mu.Unlock()

	for key, entry := range krl.limiters {
		if entry.lastAccess.Before(threshold) {
			delete(krl.limiters, key)
		}
	}
}

// Len returns the current number of active limiters (for monitoring/testing).
func (krl *KeyedRateLimiter) Len() int {
	krl.mu.RLock()
	defer krl.mu.RUnlock()
	return len(krl.limiters)
}
