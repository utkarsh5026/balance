package health

import (
	"testing"
	"time"

	"github.com/utkarsh5026/balance/pkg/node"
)

// TestPassiveCheckerDefaults tests that defaults are set correctly
func TestPassiveCheckerDefaults(t *testing.T) {
	config := passiveCheckerConfig{}
	checker := NewPassiveChecker(config)

	if checker.config.ErrorRateThreshold != 0.5 {
		t.Errorf("ErrorRateThreshold = %f, want %f", checker.config.ErrorRateThreshold, 0.5)
	}
	if checker.config.MinRequests != 10 {
		t.Errorf("MinRequests = %d, want %d", checker.config.MinRequests, 10)
	}
	if checker.config.ConsecutiveFailures != 5 {
		t.Errorf("ConsecutiveFailures = %d, want %d", checker.config.ConsecutiveFailures, 5)
	}
	if checker.config.Window != 1*time.Minute {
		t.Errorf("Window = %v, want %v", checker.config.Window, 1*time.Minute)
	}
}

// TestPassiveCheckerRecordSuccess tests recording successes
func TestPassiveCheckerRecordSuccess(t *testing.T) {
	config := passiveCheckerConfig{
		ErrorRateThreshold:  0.5,
		MinRequests:         10,
		ConsecutiveFailures: 5,
		Window:              1 * time.Minute,
	}
	checker := NewPassiveChecker(config)

	n := node.NewNode("test", "localhost:8080", 1)

	// Record multiple successes
	for i := 0; i < 10; i++ {
		checker.RecordSuccess(n, 100*time.Millisecond)
	}

	tracker := checker.getTracker(n)
	if tracker.consecutiveFailures.Load() != 0 {
		t.Errorf("consecutiveFailures = %d, want %d", tracker.consecutiveFailures.Load(), 0)
	}

	// Count total requests
	var totalReq int64
	for _, bucket := range tracker.buckets {
		totalReq += bucket.totalReq
	}
	if totalReq != 10 {
		t.Errorf("totalReq = %d, want %d", totalReq, 10)
	}
}

// TestPassiveCheckerRecordFailure tests recording failures
func TestPassiveCheckerRecordFailure(t *testing.T) {
	config := passiveCheckerConfig{
		ErrorRateThreshold:  0.5,
		MinRequests:         10,
		ConsecutiveFailures: 5,
		Window:              1 * time.Minute,
	}
	checker := NewPassiveChecker(config)

	n := node.NewNode("test", "localhost:8080", 1)

	// Record some successes first
	for i := 0; i < 5; i++ {
		checker.RecordSuccess(n, 100*time.Millisecond)
	}

	// Record a failure
	shouldMarkUnhealthy := checker.RecordFailure(n, nil)
	if shouldMarkUnhealthy {
		t.Error("should not mark unhealthy after one failure")
	}

	tracker := checker.getTracker(n)
	if tracker.consecutiveFailures.Load() != 1 {
		t.Errorf("consecutiveFailures = %d, want %d", tracker.consecutiveFailures.Load(), 1)
	}
}

// TestPassiveCheckerConsecutiveFailures tests consecutive failure threshold
func TestPassiveCheckerConsecutiveFailures(t *testing.T) {
	config := passiveCheckerConfig{
		ErrorRateThreshold:  0.5,
		MinRequests:         100,
		ConsecutiveFailures: 5,
		Window:              1 * time.Minute,
	}
	checker := NewPassiveChecker(config)

	n := node.NewNode("test", "localhost:8080", 1)

	// Record failures below threshold
	for i := 0; i < 4; i++ {
		shouldMarkUnhealthy := checker.RecordFailure(n, nil)
		if shouldMarkUnhealthy {
			t.Errorf("should not mark unhealthy at failure %d", i+1)
		}
	}

	// Record one more failure to reach threshold
	shouldMarkUnhealthy := checker.RecordFailure(n, nil)
	if !shouldMarkUnhealthy {
		t.Error("should mark unhealthy after 5 consecutive failures")
	}
}

// TestPassiveCheckerErrorRate tests error rate threshold
func TestPassiveCheckerErrorRate(t *testing.T) {
	config := passiveCheckerConfig{
		ErrorRateThreshold:  0.5, // 50% error rate
		MinRequests:         10,
		ConsecutiveFailures: 100, // Set high so only error rate triggers
		Window:              1 * time.Minute,
	}
	checker := NewPassiveChecker(config)

	n := node.NewNode("test", "localhost:8080", 1)

	// Record 6 successes and 4 failures (40% error rate - below threshold)
	for i := 0; i < 6; i++ {
		checker.RecordSuccess(n, 100*time.Millisecond)
	}
	for i := 0; i < 4; i++ {
		shouldMarkUnhealthy := checker.RecordFailure(n, nil)
		if shouldMarkUnhealthy {
			t.Errorf("should not mark unhealthy at 40%% error rate")
		}
	}

	// Record 2 more failures (6 successes, 6 failures = 50% error rate)
	shouldMarkUnhealthy := checker.RecordFailure(n, nil)
	if shouldMarkUnhealthy {
		t.Error("should not mark unhealthy at exactly 50% error rate (threshold is >=)")
	}

	// Record one more failure (6 successes, 7 failures = 53.8% error rate)
	shouldMarkUnhealthy = checker.RecordFailure(n, nil)
	if !shouldMarkUnhealthy {
		t.Error("should mark unhealthy when error rate exceeds threshold")
	}
}

// TestPassiveCheckerMinRequests tests minimum requests requirement
func TestPassiveCheckerMinRequests(t *testing.T) {
	config := passiveCheckerConfig{
		ErrorRateThreshold:  0.5,
		MinRequests:         10,
		ConsecutiveFailures: 100, // Set high so only error rate triggers
		Window:              1 * time.Minute,
	}
	checker := NewPassiveChecker(config)

	n := node.NewNode("test", "localhost:8080", 1)

	// Record 9 failures (below MinRequests)
	for i := 0; i < 9; i++ {
		shouldMarkUnhealthy := checker.RecordFailure(n, nil)
		if shouldMarkUnhealthy {
			t.Errorf("should not mark unhealthy with only %d requests", i+1)
		}
	}

	// Even though error rate is 100%, should not trigger until MinRequests is reached
	tracker := checker.getTracker(n)
	var totalReq int64
	for _, bucket := range tracker.buckets {
		totalReq += bucket.totalReq
	}
	if totalReq >= 10 {
		t.Errorf("totalReq = %d, want < 10", totalReq)
	}
}

// TestPassiveCheckerReset tests resetting failure tracking
func TestPassiveCheckerReset(t *testing.T) {
	config := passiveCheckerConfig{
		ErrorRateThreshold:  0.5,
		MinRequests:         10,
		ConsecutiveFailures: 5,
		Window:              1 * time.Minute,
	}
	checker := NewPassiveChecker(config)

	n := node.NewNode("test", "localhost:8080", 1)

	// Record some failures
	for i := 0; i < 3; i++ {
		checker.RecordFailure(n, nil)
	}

	tracker := checker.getTracker(n)
	if tracker.consecutiveFailures.Load() != 3 {
		t.Errorf("consecutiveFailures before reset = %d, want %d", tracker.consecutiveFailures.Load(), 3)
	}

	// Reset
	checker.Reset(n)

	// Getting tracker again should create a new one
	tracker = checker.getTracker(n)
	if tracker.consecutiveFailures.Load() != 0 {
		t.Errorf("consecutiveFailures after reset = %d, want %d", tracker.consecutiveFailures.Load(), 0)
	}
}

// TestPassiveCheckerSuccessResetsConsecutiveFailures tests that success resets consecutive failures
func TestPassiveCheckerSuccessResetsConsecutiveFailures(t *testing.T) {
	config := passiveCheckerConfig{
		ErrorRateThreshold:  0.5,
		MinRequests:         10,
		ConsecutiveFailures: 5,
		Window:              1 * time.Minute,
	}
	checker := NewPassiveChecker(config)

	n := node.NewNode("test", "localhost:8080", 1)

	// Record some failures
	for i := 0; i < 3; i++ {
		checker.RecordFailure(n, nil)
	}

	tracker := checker.getTracker(n)
	if tracker.consecutiveFailures.Load() != 3 {
		t.Errorf("consecutiveFailures = %d, want %d", tracker.consecutiveFailures.Load(), 3)
	}

	// Record a success
	checker.RecordSuccess(n, 100*time.Millisecond)

	if tracker.consecutiveFailures.Load() != 0 {
		t.Errorf("consecutiveFailures after success = %d, want %d", tracker.consecutiveFailures.Load(), 0)
	}
}

// TestPassiveCheckerTimeBuckets tests time bucket rotation
func TestPassiveCheckerTimeBuckets(t *testing.T) {
	config := passiveCheckerConfig{
		ErrorRateThreshold:  0.5,
		MinRequests:         10,
		ConsecutiveFailures: 100,
		Window:              160 * time.Millisecond, // 16 buckets * 10ms each
	}
	checker := NewPassiveChecker(config)

	n := node.NewNode("test", "localhost:8080", 1)

	// Record some requests
	for i := 0; i < 5; i++ {
		checker.RecordSuccess(n, 100*time.Millisecond)
	}

	tracker := checker.getTracker(n)
	initialBucketIdx := tracker.bucketIdx

	// Wait for bucket rotation
	time.Sleep(15 * time.Millisecond)

	// Record more requests
	for i := 0; i < 5; i++ {
		checker.RecordFailure(n, nil)
	}

	// Bucket index should have changed
	if tracker.bucketIdx == initialBucketIdx {
		t.Error("bucket index should have changed after time elapsed")
	}
}

// TestPassiveCheckerCleanupOldBuckets tests that old buckets are cleaned up
func TestPassiveCheckerCleanupOldBuckets(t *testing.T) {
	config := passiveCheckerConfig{
		ErrorRateThreshold:  0.5,
		MinRequests:         5,
		ConsecutiveFailures: 100,
		Window:              160 * time.Millisecond,
	}
	checker := NewPassiveChecker(config)

	n := node.NewNode("test", "localhost:8080", 1)

	// Record requests
	for i := 0; i < 10; i++ {
		checker.RecordSuccess(n, 100*time.Millisecond)
	}

	tracker := checker.getTracker(n)
	var totalReq int64
	for _, bucket := range tracker.buckets {
		totalReq += bucket.totalReq
	}
	if totalReq != 10 {
		t.Errorf("totalReq = %d, want %d", totalReq, 10)
	}

	// Wait for window to expire
	time.Sleep(200 * time.Millisecond)

	// Record a new request (should clean up old buckets)
	checker.RecordSuccess(n, 100*time.Millisecond)

	tracker = checker.getTracker(n)
	totalReq = 0
	for _, bucket := range tracker.buckets {
		totalReq += bucket.totalReq
	}

	// Only the new request should remain
	if totalReq != 1 {
		t.Errorf("totalReq after cleanup = %d, want %d", totalReq, 1)
	}
}

// TestPassiveCheckerConcurrency tests concurrent access
func TestPassiveCheckerConcurrency(t *testing.T) {
	config := passiveCheckerConfig{
		ErrorRateThreshold:  0.5,
		MinRequests:         100,
		ConsecutiveFailures: 50,
		Window:              1 * time.Minute,
	}
	checker := NewPassiveChecker(config)

	n := node.NewNode("test", "localhost:8080", 1)

	// Concurrent successes and failures
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				checker.RecordSuccess(n, 100*time.Millisecond)
			}
			done <- true
		}()
		go func() {
			for j := 0; j < 10; j++ {
				checker.RecordFailure(n, nil)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	tracker := checker.getTracker(n)
	var totalReq, failedReq int64
	for _, bucket := range tracker.buckets {
		totalReq += bucket.totalReq
		failedReq += bucket.failedReq
	}

	if totalReq != 200 {
		t.Errorf("totalReq = %d, want %d", totalReq, 200)
	}
	if failedReq != 100 {
		t.Errorf("failedReq = %d, want %d", failedReq, 100)
	}
}

// TestPassiveCheckerMultipleBackends tests tracking multiple backends
func TestPassiveCheckerMultipleBackends(t *testing.T) {
	config := passiveCheckerConfig{
		ErrorRateThreshold:  0.5,
		MinRequests:         10,
		ConsecutiveFailures: 5,
		Window:              1 * time.Minute,
	}
	checker := NewPassiveChecker(config)

	n1 := node.NewNode("backend1", "localhost:8081", 1)
	n2 := node.NewNode("backend2", "localhost:8082", 1)

	// Record different patterns for each backend
	for i := 0; i < 10; i++ {
		checker.RecordSuccess(n1, 100*time.Millisecond)
	}
	for i := 0; i < 10; i++ {
		checker.RecordFailure(n2, nil)
	}

	tracker1 := checker.getTracker(n1)
	tracker2 := checker.getTracker(n2)

	if tracker1.consecutiveFailures.Load() != 0 {
		t.Errorf("backend1 consecutiveFailures = %d, want %d", tracker1.consecutiveFailures.Load(), 0)
	}
	if tracker2.consecutiveFailures.Load() != 10 {
		t.Errorf("backend2 consecutiveFailures = %d, want %d", tracker2.consecutiveFailures.Load(), 10)
	}
}
