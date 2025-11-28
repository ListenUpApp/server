package api

import (
	"net/http"
	"sync"
	"time"

	"github.com/listenupapp/listenup-server/internal/http/response"
)

// RateLimiter implements a simple in-memory rate limiter using the token bucket algorithm.
// For self-hosted deployments, this provides sensible default protection against brute-force
// attacks without requiring external dependencies like Redis.
type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	rate     int           // tokens per interval
	interval time.Duration // refill interval
	burst    int           // max tokens (bucket size)
}

type bucket struct {
	tokens     int
	lastRefill time.Time
}

// NewRateLimiter creates a new rate limiter.
// rate: number of requests allowed per interval
// interval: time period for rate (e.g., time.Minute)
// burst: maximum burst size (allows short bursts above rate).
func NewRateLimiter(rate int, interval time.Duration, burst int) *RateLimiter {
	rl := &RateLimiter{
		buckets:  make(map[string]*bucket),
		rate:     rate,
		interval: interval,
		burst:    burst,
	}

	// Cleanup old buckets periodically to prevent memory leaks.
	go rl.cleanup()

	return rl
}

// Allow checks if a request from the given key should be allowed.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, exists := rl.buckets[key]

	if !exists {
		// New client, create bucket with full tokens.
		rl.buckets[key] = &bucket{
			tokens:     rl.burst - 1, // Use one token for this request
			lastRefill: now,
		}
		return true
	}

	// Refill tokens based on elapsed time.
	elapsed := now.Sub(b.lastRefill)
	tokensToAdd := int(elapsed / rl.interval) * rl.rate

	if tokensToAdd > 0 {
		b.tokens = min(b.tokens+tokensToAdd, rl.burst)
		b.lastRefill = now
	}

	// Check if we have tokens available.
	if b.tokens > 0 {
		b.tokens--
		return true
	}

	return false
}

// cleanup removes stale buckets to prevent memory growth.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		staleThreshold := 30 * time.Minute

		for key, b := range rl.buckets {
			if now.Sub(b.lastRefill) > staleThreshold {
				delete(rl.buckets, key)
			}
		}
		rl.mu.Unlock()
	}
}

// RateLimitMiddleware creates a middleware that rate limits requests by IP.
// Returns 429 Too Many Requests when limit is exceeded.
func RateLimitMiddleware(limiter *RateLimiter, logger interface{ Warn(msg string, args ...any) }) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Use IP address as the rate limit key.
			key := getClientIP(r)

			if !limiter.Allow(key) {
				logger.Warn("Rate limit exceeded",
					"ip", key,
					"path", r.URL.Path,
				)
				response.TooManyRequests(w, "Too many requests. Please try again later.", nil)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// getClientIP extracts the client IP from the request.
// Checks X-Forwarded-For and X-Real-IP headers before falling back to RemoteAddr.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For (may contain multiple IPs, first is client).
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take first IP in the chain.
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}

	// Check X-Real-IP.
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr (strip port).
	ip := r.RemoteAddr
	for i := len(ip) - 1; i >= 0; i-- {
		if ip[i] == ':' {
			return ip[:i]
		}
	}
	return ip
}
