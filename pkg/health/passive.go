package health

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/utkarsh5026/balance/pkg/node"
)

const (
	defaultBuckets = 16
)

// passiveCheckerConfig configures a passive health checker
type passiveCheckerConfig struct {
	// ErrorRateThreshold is the error rate (0.0-1.0) that triggers unhealthy
	ErrorRateThreshold float64

	// MinRequests is the minimum number of requests before checking error rate
	MinRequests int64

	// ConsecutiveFailures is the number of consecutive failures to mark unhealthy
	ConsecutiveFailures int

	// Window is the time window for tracking failures
	Window time.Duration
}

// passiveChecker monitors backend failures and marks them unhealthy
type passiveChecker struct {
	config passiveCheckerConfig

	// Track failures per backend
	failures map[*node.Node]*failureTracker
	mu       sync.RWMutex
}

type timeBucket struct {
	totalReq, failedReq int64
}

// failureTracker tracks failures for a single backend
type failureTracker struct {
	consecutiveFailures atomic.Int64
	buckets             []timeBucket
	bucketIdx           int // Current bucket index
	bucketDuration      time.Duration
	lastAccess          time.Time
	mu                  sync.Mutex
}

// NewPassiveChecker creates a new passive health checker
func NewPassiveChecker(config passiveCheckerConfig) *passiveChecker {
	if config.ErrorRateThreshold == 0 {
		config.ErrorRateThreshold = 0.5
	}
	if config.MinRequests == 0 {
		config.MinRequests = 10
	}
	if config.ConsecutiveFailures == 0 {
		config.ConsecutiveFailures = 5
	}
	if config.Window == 0 {
		config.Window = 1 * time.Minute
	}

	return &passiveChecker{
		config:   config,
		failures: make(map[*node.Node]*failureTracker),
	}
}

func (pc *passiveChecker) RecordSuccess(n *node.Node, responseTime time.Duration) {
	t := pc.getTracker(n)

	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	pc.cleanupOld(t, now)

	t.consecutiveFailures.Store(0)
	t.buckets[t.bucketIdx].totalReq++
}

func (pc *passiveChecker) RecordFailure(n *node.Node, err error) (nodeHealthy bool) {
	t := pc.getTracker(n)
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	pc.cleanupOld(t, now)

	t.consecutiveFailures.Add(1)
	t.buckets[t.bucketIdx].totalReq++
	t.buckets[t.bucketIdx].failedReq++

	return pc.shouldMarkUnhealthy(t)
}

func (pc *passiveChecker) getTracker(n *node.Node) *failureTracker {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	tracker, exists := pc.failures[n]
	if !exists {
		tracker = &failureTracker{
			buckets:        make([]timeBucket, defaultBuckets),
			bucketDuration: pc.config.Window / time.Duration(defaultBuckets),
			lastAccess:     time.Now(),
		}
		pc.failures[n] = tracker
	}
	return tracker
}

func (pc *passiveChecker) cleanupOld(t *failureTracker, now time.Time) {
	elapsed := now.Sub(t.lastAccess)
	if elapsed < t.bucketDuration {
		return
	}

	skipCount := int(elapsed / t.bucketDuration)
	if skipCount >= len(t.buckets) {
		for i := range t.buckets {
			t.buckets[i] = timeBucket{}
		}
		t.lastAccess = now
		return
	}

	for range skipCount {
		t.bucketIdx = (t.bucketIdx + 1) % len(t.buckets)
		t.buckets[t.bucketIdx] = timeBucket{}
	}
	t.lastAccess = now
}

func (pc *passiveChecker) shouldMarkUnhealthy(tracker *failureTracker) bool {
	if tracker.consecutiveFailures.Load() >= int64(pc.config.ConsecutiveFailures) {
		return true
	}

	var totalReq, failedReq int64
	for _, bucket := range tracker.buckets {
		totalReq += bucket.totalReq
		failedReq += bucket.failedReq
	}

	if totalReq >= pc.config.MinRequests {
		errorRate := float64(failedReq) / float64(totalReq)
		if errorRate >= pc.config.ErrorRateThreshold {
			return true
		}
	}

	return false
}

func (pc *passiveChecker) Reset(n *node.Node) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	delete(pc.failures, n)
}
