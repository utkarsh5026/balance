package security

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewTokenBucket(t *testing.T) {
	ctx := context.Background()
	rate := 10.0
	capacity := int64(100)

	tb := NewTokenBucket(ctx, rate, capacity)
	if tb == nil {
		t.Fatal("NewTokenBucket returned nil")
	}

	if tb.rate != rate {
		t.Errorf("expected rate %f, got %f", rate, tb.rate)
	}

	if tb.capacity != capacity {
		t.Errorf("expected capacity %d, got %d", capacity, tb.capacity)
	}

	if tb.buckets == nil {
		t.Error("buckets map should be initialized")
	}

	if tb.cleanupInterval != 1*time.Minute {
		t.Errorf("expected cleanupInterval 1m, got %v", tb.cleanupInterval)
	}

	if tb.bucketTTL != 5*time.Minute {
		t.Errorf("expected bucketTTL 5m, got %v", tb.bucketTTL)
	}
}

func TestTokenBucket_Allow_NewKey(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tb := NewTokenBucket(ctx, 10.0, 5)
	key := "test-key"

	// First request should be allowed (bucket starts full)
	if !tb.Allow(key) {
		t.Error("first request should be allowed")
	}

	// Verify bucket was created
	tb.mu.RLock()
	_, exists := tb.buckets[key]
	tb.mu.RUnlock()

	if !exists {
		t.Error("bucket should exist after first Allow call")
	}
}

func TestTokenBucket_Allow_Capacity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	capacity := int64(5)
	tb := NewTokenBucket(ctx, 0.0, capacity) // 0 rate means no refill

	key := "test-key"

	// Should allow exactly 'capacity' requests
	for i := 0; i < int(capacity); i++ {
		if !tb.Allow(key) {
			t.Errorf("request %d should be allowed (capacity: %d)", i+1, capacity)
		}
	}

	// Next request should be denied
	if tb.Allow(key) {
		t.Error("request beyond capacity should be denied")
	}
}

func TestTokenBucket_Allow_Refill(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rate := 10.0 // 10 tokens per second
	capacity := int64(5)
	tb := NewTokenBucket(ctx, rate, capacity)

	key := "test-key"

	// Exhaust the bucket
	for i := 0; i < int(capacity); i++ {
		tb.Allow(key)
	}

	// Should be denied now
	if tb.Allow(key) {
		t.Error("should be denied after exhausting bucket")
	}

	// Wait for 200ms (should refill ~2 tokens at 10 tokens/sec)
	time.Sleep(200 * time.Millisecond)

	// Should allow at least 1 request after refill
	if !tb.Allow(key) {
		t.Error("should allow at least 1 request after refill")
	}

	// Second request should also be allowed (we refilled ~2 tokens)
	if !tb.Allow(key) {
		t.Error("should allow second request after refill")
	}

	// If we make requests faster than the rate, should be denied
	// Make 10 requests in rapid succession
	allowed := 0
	for i := 0; i < 10; i++ {
		if tb.Allow(key) {
			allowed++
		}
	}

	// With a rate of 10/sec and capacity of 5, after using 2,
	// we should not be able to use 10 more immediately
	if allowed >= 10 {
		t.Error("should not allow 10 rapid requests after partial refill")
	}
}

func TestTokenBucket_Allow_MaxCapacity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rate := 1000.0 // High rate
	capacity := int64(10)
	tb := NewTokenBucket(ctx, rate, capacity)

	key := "test-key"

	// Use one token
	tb.Allow(key)

	// Wait long enough to refill many tokens
	time.Sleep(100 * time.Millisecond)

	// Should only have capacity tokens, not more
	count := 0
	for tb.Allow(key) {
		count++
		if count > int(capacity)+1 {
			t.Errorf("allowed more than capacity: got %d, max %d", count, capacity)
			break
		}
	}

	if count != int(capacity) {
		t.Errorf("expected to allow %d requests (capacity), got %d", capacity, count)
	}
}

func TestTokenBucket_Allow_MultipleKeys(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tb := NewTokenBucket(ctx, 10.0, 5)

	key1 := "key1"
	key2 := "key2"

	// Exhaust key1
	for i := 0; i < 5; i++ {
		tb.Allow(key1)
	}

	// key1 should be denied
	if tb.Allow(key1) {
		t.Error("key1 should be denied after exhaustion")
	}

	// key2 should still be allowed (separate bucket)
	if !tb.Allow(key2) {
		t.Error("key2 should be allowed (independent bucket)")
	}
}

func TestTokenBucket_Reset(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tb := NewTokenBucket(ctx, 0.0, 5)
	key := "test-key"

	// Exhaust the bucket
	for i := 0; i < 5; i++ {
		tb.Allow(key)
	}

	// Should be denied
	if tb.Allow(key) {
		t.Error("should be denied after exhaustion")
	}

	// Reset the bucket
	if !tb.Reset(key) {
		t.Error("Reset should return true for existing key")
	}

	// Should be allowed again (new bucket with full capacity)
	if !tb.Allow(key) {
		t.Error("should be allowed after reset")
	}
}

func TestTokenBucket_Reset_NonExistentKey(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tb := NewTokenBucket(ctx, 10.0, 5)

	// Reset non-existent key
	if tb.Reset("non-existent") {
		t.Error("Reset should return false for non-existent key")
	}
}

func TestTokenBucket_Cleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tb := &TokenBucket{
		rate:            10.0,
		capacity:        5,
		buckets:         make(map[string]*bucket),
		cleanupInterval: 100 * time.Millisecond,
		bucketTTL:       200 * time.Millisecond,
	}
	go tb.cleanupPeriodically(ctx)

	key1 := "key1"
	key2 := "key2"

	// Create buckets
	tb.Allow(key1)
	tb.Allow(key2)

	// Verify both exist
	tb.mu.RLock()
	count := len(tb.buckets)
	tb.mu.RUnlock()

	if count != 2 {
		t.Errorf("expected 2 buckets, got %d", count)
	}

	// Wait for TTL to expire
	time.Sleep(300 * time.Millisecond)

	// Wait for cleanup to run
	time.Sleep(150 * time.Millisecond)

	// Buckets should be cleaned up
	tb.mu.RLock()
	count = len(tb.buckets)
	tb.mu.RUnlock()

	if count != 0 {
		t.Errorf("expected 0 buckets after cleanup, got %d", count)
	}
}

func TestTokenBucket_CleanupKeepsActive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tb := &TokenBucket{
		rate:            10.0,
		capacity:        5,
		buckets:         make(map[string]*bucket),
		cleanupInterval: 100 * time.Millisecond,
		bucketTTL:       300 * time.Millisecond,
	}
	go tb.cleanupPeriodically(ctx)

	key := "active-key"

	// Create bucket
	tb.Allow(key)

	// Keep using it
	for i := 0; i < 5; i++ {
		time.Sleep(100 * time.Millisecond)
		tb.Allow(key)
	}

	// Bucket should still exist (active)
	tb.mu.RLock()
	_, exists := tb.buckets[key]
	tb.mu.RUnlock()

	if !exists {
		t.Error("active bucket should not be cleaned up")
	}
}

func TestTokenBucket_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create with short cleanup interval for testing
	tb := &TokenBucket{
		rate:            10.0,
		capacity:        5,
		buckets:         make(map[string]*bucket),
		cleanupInterval: 50 * time.Millisecond,
		bucketTTL:       5 * time.Minute,
	}
	go tb.cleanupPeriodically(ctx)

	// Cancel context
	cancel()

	// Wait a bit to ensure cleanup goroutine exits
	time.Sleep(100 * time.Millisecond)

	// Allow should still work even after context cancellation
	if !tb.Allow("test-key") {
		t.Error("Allow should work even after context cancellation")
	}
}

func TestTokenBucket_ConcurrentAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tb := NewTokenBucket(ctx, 100.0, 1000)
	key := "concurrent-key"

	var wg sync.WaitGroup
	goroutines := 100
	requestsPerGoroutine := 10

	allowed := make([]int, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			count := 0
			for j := 0; j < requestsPerGoroutine; j++ {
				if tb.Allow(key) {
					count++
				}
			}
			allowed[idx] = count
		}(i)
	}

	wg.Wait()

	// Count total allowed
	total := 0
	for _, count := range allowed {
		total += count
	}

	// Should not exceed capacity initially + refills during execution
	// With 100 tokens/sec and brief execution, should be close to capacity (1000)
	if total > 1100 {
		t.Errorf("allowed too many requests under concurrent load: %d", total)
	}

	// Should allow at least the capacity
	if total < 1000 {
		t.Logf("warning: allowed fewer than capacity (%d < 1000), might indicate race condition", total)
	}
}

func TestTokenBucket_ConcurrentReset(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tb := NewTokenBucket(ctx, 10.0, 100)
	key := "reset-key"

	var wg sync.WaitGroup

	// Concurrent Allow and Reset operations
	for i := 0; i < 50; i++ {
		wg.Add(2)

		go func() {
			defer wg.Done()
			tb.Allow(key)
		}()

		go func() {
			defer wg.Done()
			tb.Reset(key)
		}()
	}

	wg.Wait()

	// Should not panic and should still be functional
	if !tb.Allow(key) {
		t.Error("should be allowed after concurrent operations")
	}
}

func TestTokenBucket_ZeroTokens(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tb := NewTokenBucket(ctx, 0.0, 10)
	key := "test-key"

	// Exhaust all tokens
	for i := 0; i < 10; i++ {
		tb.Allow(key)
	}

	// Verify exactly 0 tokens
	tb.mu.RLock()
	b := tb.buckets[key]
	tokens := b.tokens
	tb.mu.RUnlock()

	if tokens != 0 {
		t.Errorf("expected 0 tokens, got %f", tokens)
	}

	// Should be denied
	if tb.Allow(key) {
		t.Error("should deny when tokens = 0")
	}
}

func TestTokenBucket_FractionalRefill(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rate := 2.5 // 2.5 tokens per second
	capacity := int64(10)
	tb := NewTokenBucket(ctx, rate, capacity)

	key := "test-key"

	// Exhaust bucket
	for i := 0; i < int(capacity); i++ {
		tb.Allow(key)
	}

	// Verify it's exhausted
	if tb.Allow(key) {
		t.Error("should be denied when exhausted")
	}

	// Wait 400ms (should refill ~1 token at 2.5 tokens/sec)
	time.Sleep(400 * time.Millisecond)

	// Should allow exactly 1 request
	if !tb.Allow(key) {
		t.Error("should allow 1 request after partial refill")
	}

	// Make multiple rapid requests - should not allow many
	allowed := 0
	for i := 0; i < 10; i++ {
		if tb.Allow(key) {
			allowed++
		}
	}

	// Should allow very few (tokens refill slowly at 2.5/sec)
	if allowed > 2 {
		t.Errorf("should allow max 2 rapid requests, got %d", allowed)
	}
}

func TestTokenBucket_NegativeTokensBug(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tb := NewTokenBucket(ctx, 1.0, 10) // 1 token/sec
	key := "test-key"

	// Exhaust all tokens
	for i := 0; i < 10; i++ {
		tb.Allow(key)
	}

	// Should be denied
	if tb.Allow(key) {
		t.Error("should deny when exhausted")
	}

	// Wait for 0.5 seconds (refills 0.5 tokens)
	time.Sleep(500 * time.Millisecond)

	// First allow will have 0.5 tokens
	// This is the bug: 0.5 != 0, so it passes, then becomes -0.5
	tb.mu.Lock()
	b := tb.buckets[key]
	tb.refill(b)
	tokensBefore := b.tokens
	tb.mu.Unlock()

	t.Logf("Tokens before Allow: %f", tokensBefore)

	result := tb.Allow(key)
	t.Logf("Allow result: %v", result)

	tb.mu.Lock()
	tokensAfter := b.tokens
	tb.mu.Unlock()

	t.Logf("Tokens after Allow: %f", tokensAfter)

	// The bug: allows request even with < 1 token, resulting in negative tokens
	if result && tokensAfter < 0 {
		t.Error("BUG CONFIRMED: Allowed request with < 1 token, resulting in negative tokens")
	}
}

func BenchmarkTokenBucket_Allow(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tb := NewTokenBucket(ctx, 1000000.0, 1000000)
	key := "bench-key"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tb.Allow(key)
	}
}

func BenchmarkTokenBucket_AllowMultipleKeys(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tb := NewTokenBucket(ctx, 1000000.0, 1000000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := string(rune(i % 100)) // 100 different keys
		tb.Allow(key)
	}
}

func BenchmarkTokenBucket_AllowParallel(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tb := NewTokenBucket(ctx, 1000000.0, 1000000)
	key := "bench-key"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			tb.Allow(key)
		}
	})
}
