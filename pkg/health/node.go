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

type nodeWithHealth struct {
	Node  *node.Node
	State atomic.Value // NodeState

	Metrics *HealthMetrics

	// Thresholds for state transitions
	HealthyThreshold   int
	UnhealthyThreshold int

	mu        sync.RWMutex
	listeners []StateChangeListener
}

func NewNodeWithHealth(n *node.Node, healthyThreshold, unhealthyThreshold int) *nodeWithHealth {
	nh := &nodeWithHealth{
		Node:               n,
		HealthyThreshold:   healthyThreshold,
		UnhealthyThreshold: unhealthyThreshold,
		listeners:          make([]StateChangeListener, 0),
		Metrics:            &HealthMetrics{},
	}
	nh.State.Store(StateHealthy)
	now := time.Now()
	nh.Metrics.lastCheckTime.Store(now)
	nh.Metrics.lastStateChange.Store(now)
	return nh
}

func (h *nodeWithHealth) RecordSuccess() {

	h.Metrics.consecutiveSuccesses.Add(1)
	h.Metrics.consecutiveFailures.Store(0)
	h.Metrics.totalSuccesses.Add(1)
	h.Metrics.lastCheckTime.Store(time.Now())

	// Check if we should transition to healthy
	if h.Metrics.consecutiveSuccesses.Load() >= int64(h.HealthyThreshold) {
		h.transitionTo(StateHealthy)
	}
}

func (h *nodeWithHealth) RecordFailure() {
	h.Metrics.consecutiveFailures.Add(1)
	h.Metrics.consecutiveSuccesses.Store(0)
	h.Metrics.totalFailures.Add(1)
	h.Metrics.lastCheckTime.Store(time.Now())

	if h.Metrics.consecutiveFailures.Load() >= int64(h.UnhealthyThreshold) {
		h.transitionTo(StateUnhealthy)
	}
}

func (h *nodeWithHealth) RecordRequest(success bool, responseTime time.Duration) {
	h.Metrics.totalRequests.Add(1)
	h.Metrics.totalResponseTime.Add(responseTime.Nanoseconds())
	if success {
		h.Metrics.totalResponseTime.Add(responseTime.Nanoseconds())
	} else {
		h.Metrics.failedRequests.Add(1)
	}
}

func (h *nodeWithHealth) transitionTo(newState NodeState) {
	oldState := h.State.Load().(NodeState)
	if oldState == newState {
		return
	}

	h.State.Store(newState)
	h.Metrics.lastStateChange.Store(time.Now())

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

func (h *nodeWithHealth) AddListener(listener StateChangeListener) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.listeners = append(h.listeners, listener)
}
