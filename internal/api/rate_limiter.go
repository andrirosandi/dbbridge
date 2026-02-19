package api

import (
	"dbbridge/internal/logger"
	"net"
	"net/http"
	"sync"
	"time"
)

// RateLimiter implements a simple in-memory token bucket rate limiter.
// Each unique key (IP or API key) gets its own bucket.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64       // tokens per second
	burst   int           // max tokens (burst capacity)
	cleanup time.Duration // how often to prune stale entries
}

type bucket struct {
	tokens    float64
	lastCheck time.Time
}

// NewRateLimiter creates a rate limiter.
// rate = requests per minute, burst = max burst size.
func NewRateLimiter(ratePerMinute float64, burst int) *RateLimiter {
	rl := &RateLimiter{
		buckets: make(map[string]*bucket),
		rate:    ratePerMinute / 60.0, // convert to per-second
		burst:   burst,
		cleanup: 5 * time.Minute,
	}

	// Start cleanup goroutine
	go rl.cleanupLoop()

	return rl
}

// Allow checks if a request from the given key is allowed.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, exists := rl.buckets[key]

	if !exists {
		// New key — start with full bucket minus 1 token
		rl.buckets[key] = &bucket{
			tokens:    float64(rl.burst) - 1,
			lastCheck: now,
		}
		return true
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(b.lastCheck).Seconds()
	b.tokens += elapsed * rl.rate
	if b.tokens > float64(rl.burst) {
		b.tokens = float64(rl.burst)
	}
	b.lastCheck = now

	if b.tokens >= 1 {
		b.tokens--
		return true
	}

	return false
}

// Middleware returns a Chi-compatible middleware that rate limits by IP.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := extractIP(r)

		if !rl.Allow(key) {
			logger.Info.Printf("Rate limit exceeded for %s on %s", key, r.URL.Path)
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// MiddlewareByAPIKey returns a middleware that rate limits by API key (falls back to IP).
func (rl *RateLimiter) MiddlewareByAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		if key == "" {
			key = extractIP(r) // fallback to IP
		}

		if !rl.Allow(key) {
			logger.Info.Printf("Rate limit exceeded for API key/IP on %s", r.URL.Path)
			http.Error(w, `{"error":"Too Many Requests"}`, http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// extractIP gets the client IP from the request.
func extractIP(r *http.Request) string {
	// Check X-Forwarded-For first (for reverse proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Fallback to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// cleanupLoop periodically removes stale buckets.
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanup)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, b := range rl.buckets {
			// Remove if no activity for 10 minutes
			if now.Sub(b.lastCheck) > 10*time.Minute {
				delete(rl.buckets, key)
			}
		}
		rl.mu.Unlock()
	}
}
