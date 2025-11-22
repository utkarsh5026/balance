package health

import (
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/utkarsh5026/balance/pkg/node"
)

type NodeState int

const (
	// StateHealthy indicates the backend is healthy and accepting traffic
	StateHealthy NodeState = iota

	// StateUnhealthy indicates the backend has failed health checks
	StateUnhealthy

	// StateDraining indicates the backend is being gracefully removed
	StateDraining
)

// String returns the string representation of the state
func (s NodeState) String() string {
	switch s {
	case StateHealthy:
		return "healthy"
	case StateUnhealthy:
		return "unhealthy"
	case StateDraining:
		return "draining"
	default:
		return "unknown"
	}
}

// HealthMetrics tracks health-related metrics for a backend
type HealthMetrics struct {
	// Consecutive successful health checks
	consecutiveSuccesses atomic.Int64

	// Consecutive failed health checks
	consecutiveFailures atomic.Int64

	// Total successful health checks
	totalSuccesses atomic.Int64

	// Total failed health checks
	totalFailures atomic.Int64

	// Last health check time
	lastCheckTime atomic.Value // time.Time

	// Last state change time
	lastStateChange atomic.Value // time.Time

	// Total requests to this backend
	totalRequests atomic.Int64

	// Failed requests (passive health check)
	failedRequests atomic.Int64

	// Response time tracking (for passive health checks)
	totalResponseTime atomic.Int64 // in nanoseconds
}

// StateChangeListener is called when backend state changes
type StateChangeListener func(backend *node.Node, old, new NodeState)

// nodeWithHealth wraps a Node with health tracking capabilities.
// It maintains the health state and metrics for a single backend node.
type nodeWithHealth struct {
	Node  *node.Node
	State atomic.Value // NodeState

	m *HealthMetrics

	// Thresholds for state transitions
	HealthyThreshold   int
	UnhealthyThreshold int

	mu        sync.RWMutex
	listeners []StateChangeListener
}

// NewNodeWithHealth creates a new health tracking wrapper for a node.
// It initializes the health state to healthy and sets up metrics tracking.
func NewNodeWithHealth(n *node.Node, healthyThreshold, unhealthyThreshold int) *nodeWithHealth {
	nh := &nodeWithHealth{
		Node:               n,
		HealthyThreshold:   healthyThreshold,
		UnhealthyThreshold: unhealthyThreshold,
		listeners:          make([]StateChangeListener, 0),
		m:                  &HealthMetrics{},
	}
	nh.State.Store(StateHealthy)
	now := time.Now()
	nh.m.lastCheckTime.Store(now)
	nh.m.lastStateChange.Store(now)
	return nh
}

// RecordSuccess records a successful health check.
// It updates metrics and may transition the node state to healthy if the threshold is met.
func (h *nodeWithHealth) RecordSuccess() {

	h.m.consecutiveSuccesses.Add(1)
	h.m.consecutiveFailures.Store(0)
	h.m.totalSuccesses.Add(1)
	h.m.lastCheckTime.Store(time.Now())

	if h.m.consecutiveSuccesses.Load() >= int64(h.HealthyThreshold) {
		h.transitionTo(StateHealthy)
	}
}

// RecordFailure records a failed health check.
// It updates metrics and may transition the node state to unhealthy if the threshold is met.
func (h *nodeWithHealth) RecordFailure() {
	h.m.consecutiveFailures.Add(1)
	h.m.consecutiveSuccesses.Store(0)
	h.m.totalFailures.Add(1)
	h.m.lastCheckTime.Store(time.Now())

	if h.m.consecutiveFailures.Load() >= int64(h.UnhealthyThreshold) {
		h.transitionTo(StateUnhealthy)
	}
}

// RecordRequest records metrics for a request handled by this node.
// It tracks total requests, response time, and failures for passive health checking.
func (h *nodeWithHealth) RecordRequest(success bool, responseTime time.Duration) {
	h.m.totalRequests.Add(1)
	h.m.totalResponseTime.Add(responseTime.Nanoseconds())
	if !success {
		h.m.failedRequests.Add(1)
	}
}

// transitionTo changes the node's health state and notifies listeners if the state changes.
// It updates the node's health status accordingly.
func (h *nodeWithHealth) transitionTo(newState NodeState) {
	oldState := h.State.Load().(NodeState)
	if oldState == newState {
		return
	}

	h.State.Store(newState)
	h.m.lastStateChange.Store(time.Now())

	if newState == StateHealthy {
		h.Node.MarkHealthy()
	} else {
		h.Node.MarkUnhealthy()
	}

	h.mu.RLock()
	listeners := slices.Clone(h.listeners)
	h.mu.RUnlock()

	for _, listener := range listeners {
		listener(h.Node, oldState, newState)
	}
}

// AddListener adds a callback function that will be invoked when the node's state changes.
func (h *nodeWithHealth) AddListener(listener StateChangeListener) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.listeners = append(h.listeners, listener)
}
