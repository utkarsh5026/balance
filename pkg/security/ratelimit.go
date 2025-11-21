package security

import (
	"sync"
	"time"
)

// RateLimiter defines the interface for rate limiting
type RateLimiter interface {
	// Allow checks if a request should be allowed
	Allow(key string) bool

	// Reset resets the rate limiter for a specific key
	Reset(key string) bool
}

// TokenBucket implements a token bucket rate limiter
type TokenBucket struct {
	mu sync.RWMutex

	// rate is the number of tokens added per second
	rate float64

	// capacity is the maximum number of tokens
	capacity int64

	// buckets maps keys to their token buckets
	buckets map[string]*bucket

	// cleanupInterval is how often to clean up old buckets
	cleanupInterval time.Duration

	// bucketTTL is how long to keep inactive buckets
	bucketTTL time.Duration
}

// bucket represents a token bucket for a single key
type bucket struct {
	tokens     float64
	lastRefill time.Time
}

func NewTokenBucket(rate float64, capacity int64) *TokenBucket {
	tb := &TokenBucket{
		rate:            rate,
		capacity:        capacity,
		buckets:         make(map[string]*bucket),
		cleanupInterval: 1 * time.Minute,
		bucketTTL:       5 * time.Minute,
	}

	return tb
}

func (tb *TokenBucket) Allow(key string) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	b, exists := tb.buckets[key]
	if !exists {
		b = &bucket{
			tokens:     float64(tb.capacity),
			lastRefill: time.Now(),
		}
		tb.buckets[key] = b
	}
	tb.refill(b)

	if b.tokens == 0 {
		return false
	}

	b.tokens -= 1
	return true
}

func (tb *TokenBucket) refill(b *bucket) {
	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * tb.rate
	if b.tokens > float64(tb.capacity) {
		b.tokens = float64(tb.capacity)
	}
	b.lastRefill = now
}

func (tb *TokenBucket) Reset(key string) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if _, ok := tb.buckets[key]; ok {
		delete(tb.buckets, key)
		return true
	}

	return false
}
