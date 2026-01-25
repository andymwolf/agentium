// Package security provides rate limiting functionality
package security

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// RateLimiter implements a simple token bucket rate limiter
type RateLimiter struct {
	mu         sync.Mutex
	rate       int           // tokens per interval
	interval   time.Duration // time interval
	buckets    map[string]*bucket
	maxBuckets int // maximum number of buckets to track
}

type bucket struct {
	tokens    int
	lastReset time.Time
}

// NewRateLimiter creates a new rate limiter
// rate: number of requests allowed per interval
// interval: time period for the rate limit
func NewRateLimiter(rate int, interval time.Duration) *RateLimiter {
	return &RateLimiter{
		rate:       rate,
		interval:   interval,
		buckets:    make(map[string]*bucket),
		maxBuckets: 10000, // prevent memory exhaustion
	}
}

// Allow checks if a request from the given key should be allowed
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Get or create bucket
	b, exists := rl.buckets[key]
	if !exists {
		// Clean up old buckets if we're at the limit
		if len(rl.buckets) >= rl.maxBuckets {
			rl.cleanup(now)
		}

		b = &bucket{
			tokens:    rl.rate - 1, // consume one token
			lastReset: now,
		}
		rl.buckets[key] = b
		return true
	}

	// Check if we need to reset the bucket
	if now.Sub(b.lastReset) >= rl.interval {
		b.tokens = rl.rate - 1 // reset and consume one token
		b.lastReset = now
		return true
	}

	// Check if we have tokens available
	if b.tokens > 0 {
		b.tokens--
		return true
	}

	return false
}

// cleanup removes buckets that haven't been used recently
func (rl *RateLimiter) cleanup(now time.Time) {
	cutoff := now.Add(-2 * rl.interval)
	for key, b := range rl.buckets {
		if b.lastReset.Before(cutoff) {
			delete(rl.buckets, key)
		}
	}
}

// Middleware returns an HTTP middleware that applies rate limiting
func (rl *RateLimiter) Middleware(keyFunc func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFunc(r)
			if !rl.Allow(key) {
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				w.Header().Set("Retry-After", fmt.Sprintf("%d", int(rl.interval.Seconds())))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// IPKeyFunc returns a key function that uses the client's IP address
func IPKeyFunc(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxied requests)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Use the first IP in the chain
		if comma := indexOf(xff, ','); comma != -1 {
			return xff[:comma]
		}
		return xff
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	if colon := lastIndexOf(r.RemoteAddr, ':'); colon != -1 {
		return r.RemoteAddr[:colon]
	}
	return r.RemoteAddr
}

// Helper functions for string operations
func indexOf(s string, char rune) int {
	for i, c := range s {
		if c == char {
			return i
		}
	}
	return -1
}

func lastIndexOf(s string, char rune) int {
	result := -1
	for i, c := range s {
		if c == char {
			result = i
		}
	}
	return result
}