package api

import (
	"net/http"
	"time"

	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/ratelimit"
)

// RateLimiter wraps KeyedRateLimiter for API use.
// Kept for backward compatibility with existing code.
type RateLimiter = ratelimit.KeyedRateLimiter

// NewRateLimiter creates a new rate limiter.
// rate: number of requests allowed per interval
// interval: time period for rate (e.g., time.Minute)
// burst: maximum burst size
func NewRateLimiter(ratePerInterval int, interval time.Duration, burst int) *RateLimiter {
	// Convert rate/interval to requests per second
	// The old API used rate per interval, new API uses RPS
	// For example: 20 per minute = 20/60 = 0.333 rps
	rps := float64(ratePerInterval) / interval.Seconds()
	return ratelimit.New(rps, burst)
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
