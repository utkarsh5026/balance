package security

import (
	"context"
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
	mu         sync.Mutex
}

func NewTokenBucket(ctx context.Context, rate float64, capacity int64) *TokenBucket {
	tb := &TokenBucket{
		rate:            rate,
		capacity:        capacity,
		buckets:         make(map[string]*bucket),
		cleanupInterval: 1 * time.Minute,
		bucketTTL:       5 * time.Minute,
	}

	go tb.cleanupPeriodically(ctx)
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

	if b.tokens < 1 {
		return false
	}

	b.tokens -= 1
	return true
}

func (tb *TokenBucket) refill(b *bucket) {
	b.mu.Lock()
	defer b.mu.Unlock()

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

func (tb *TokenBucket) cleanupPeriodically(ctx context.Context) {
	tb.mu.RLock()
	cleanupInterval := tb.cleanupInterval
	bucketTTL := tb.bucketTTL
	tb.mu.RUnlock()

	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			tb.mu.Lock()
			now := time.Now()
			for key, b := range tb.buckets {
				b.mu.Lock()
				if now.Sub(b.lastRefill) > bucketTTL {
					delete(tb.buckets, key)
				}
				b.mu.Unlock()
			}
			tb.mu.Unlock()
		}
	}
}

type SlidingWindow struct {
	mu sync.RWMutex

	// limit is the maximum number of requests in the window
	limit int64

	// window is the time window duration
	window time.Duration

	// windows maps keys to their request windows
	windows map[string]*requestWindow

	// cleanupInterval is how often to clean up old windows
	cleanupInterval time.Duration
}

// requestWindow tracks requests in a sliding time window
type requestWindow struct {
	requests []time.Time
	mu       sync.RWMutex
}

func NewSlidingWindow(ctx context.Context, limit int64, window time.Duration) *SlidingWindow {
	sw := &SlidingWindow{
		limit:           limit,
		window:          window,
		windows:         make(map[string]*requestWindow),
		cleanupInterval: 1 * time.Minute,
	}

	go sw.cleanup(ctx)
	return sw
}

func (sw *SlidingWindow) cleanup(ctx context.Context) {
	sw.mu.RLock()
	cleanupInterval := sw.cleanupInterval
	sw.mu.RUnlock()

	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			sw.mu.Lock()

			now := time.Now()
			cutoff := now.Add(-sw.window * 2) // Keep for 2x window duration

			toDelete := make([]string, 0, len(sw.windows))
			for key, w := range sw.windows {
				w.mu.RLock()
				cnt := len(w.requests)
				if cnt == 0 || w.requests[cnt-1].Before(cutoff) {
					toDelete = append(toDelete, key)
				}
				w.mu.RUnlock()
			}

			for _, key := range toDelete {
				delete(sw.windows, key)
			}

			sw.mu.Unlock()
		}
	}
}

func (sw *SlidingWindow) Allow(key string) bool {
	sw.mu.Lock()
	w, exists := sw.windows[key]
	if !exists {
		w = &requestWindow{
			requests: make([]time.Time, 0),
		}
		sw.windows[key] = w
	}
	sw.mu.Unlock()

	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-sw.window)

	validRequests := make([]time.Time, 0)
	for _, t := range w.requests {
		if t.After(cutoff) {
			validRequests = append(validRequests, t)
		}
	}
	w.requests = validRequests

	if int64(len(w.requests)) < sw.limit {
		w.requests = append(w.requests, now)
		return true
	}

	return false
}

func (sw *SlidingWindow) Reset(key string) bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if _, ok := sw.windows[key]; ok {
		delete(sw.windows, key)
		return true
	}

	return false
}
