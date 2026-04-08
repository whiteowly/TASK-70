package httpx

import (
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// RateLimiter implements a simple in-memory per-key fixed-window rate limiter.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
	limit   int
	window  time.Duration
}

type rateBucket struct {
	count       int
	windowStart time.Time
}

// NewRateLimiter creates a rate limiter with the given limit per window.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*rateBucket),
		limit:   limit,
		window:  window,
	}
}

// Middleware returns Echo middleware that rate-limits by a key extracted via
// keyFn. If keyFn returns an empty string, the request is not rate-limited.
// This design avoids importing auth (which would create a circular dependency).
func (rl *RateLimiter) Middleware(keyFn func(c echo.Context) string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			key := keyFn(c)
			if key == "" {
				return next(c)
			}

			rl.mu.Lock()
			now := time.Now()
			b, ok := rl.buckets[key]
			if !ok || now.Sub(b.windowStart) > rl.window {
				rl.buckets[key] = &rateBucket{count: 1, windowStart: now}
				rl.mu.Unlock()
				return next(c)
			}
			if b.count >= rl.limit {
				rl.mu.Unlock()
				return NewAPIError(http.StatusTooManyRequests, "rate_limited", "Too many requests. Please try again later.")
			}
			b.count++
			rl.mu.Unlock()
			return next(c)
		}
	}
}
