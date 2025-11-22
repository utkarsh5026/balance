package health

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/utkarsh5026/balance/pkg/node"
)

// PassiveCheckerConfig configures a passive health checker
type PassiveCheckerConfig struct {
	// ErrorRateThreshold is the error rate (0.0-1.0) that triggers unhealthy
	ErrorRateThreshold float64

	// MinRequests is the minimum number of requests before checking error rate
	MinRequests int64

	// ConsecutiveFailures is the number of consecutive failures to mark unhealthy
	ConsecutiveFailures int

	// Window is the time window for tracking failures
	Window time.Duration
}

// PassiveChecker monitors backend failures and marks them unhealthy
type PassiveChecker struct {
	config PassiveCheckerConfig

	// Track failures per backend
	failures map[*node.Node]*failureTracker
	mu       sync.RWMutex
}

// failureTracker tracks failures for a single backend
type failureTracker struct {
	consecutiveFailures atomic.Int64
	lastFailureTime     time.Time
	windowFailures      []time.Time
	mu                  sync.Mutex
}

// NewPassiveChecker creates a new passive health checker
func NewPassiveChecker(config PassiveCheckerConfig) *PassiveChecker {
	if config.ErrorRateThreshold == 0 {
		config.ErrorRateThreshold = 0.5 // 50% error rate
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

	return &PassiveChecker{
		config:   config,
		failures: make(map[*node.Node]*failureTracker),
	}
}

func (pc *PassiveChecker) RecordSuccess(n *node.Node, responseTime time.Duration) {
	tracker := pc.getTracker(n)
	tracker.consecutiveFailures.Store(0)
}

func (pc *PassiveChecker) RecordFailure(n *node.Node, err error) bool {
	tracker := pc.getTracker(n)
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	now := time.Now()
	tracker.consecutiveFailures.Add(1)
	tracker.lastFailureTime = now

	tracker.windowFailures = append(tracker.windowFailures, now)

	timeStartFrom := now.Add(-pc.config.Window)
	failures := make([]time.Time, 0, len(tracker.windowFailures))

	for _, t := range tracker.windowFailures {
		if t.After(timeStartFrom) {
			failures = append(failures, t)
		}
	}

	tracker.windowFailures = failures
	return pc.shouldMarkUnhealthy(tracker)
}

func (pc *PassiveChecker) getTracker(n *node.Node) *failureTracker {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	tracker, exists := pc.failures[n]
	if !exists {
		tracker = &failureTracker{
			windowFailures: make([]time.Time, 0),
		}
		pc.failures[n] = tracker
	}
	return tracker
}

func (pc *PassiveChecker) shouldMarkUnhealthy(tracker *failureTracker) bool {
	if tracker.consecutiveFailures.Load() >= int64(pc.config.ConsecutiveFailures) {
		return true
	}
	return false
}
