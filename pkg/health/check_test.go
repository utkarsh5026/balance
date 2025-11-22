package health

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/utkarsh5026/balance/pkg/node"
)

// TestCheckerConfigValidation tests configuration validation
func TestCheckerConfigValidation(t *testing.T) {
	tests := []struct {
		name      string
		config    CheckerConfig
		wantError bool
	}{
		{
			name: "valid config with defaults",
			config: CheckerConfig{
				Interval:           10 * time.Second,
				Timeout:            3 * time.Second,
				HealthyThreshold:   2,
				UnhealthyThreshold: 3,
			},
			wantError: false,
		},
		{
			name: "negative interval",
			config: CheckerConfig{
				Interval: -1 * time.Second,
				Timeout:  3 * time.Second,
			},
			wantError: true,
		},
		{
			name: "negative timeout",
			config: CheckerConfig{
				Interval: 10 * time.Second,
				Timeout:  -1 * time.Second,
			},
			wantError: true,
		},
		{
			name: "timeout greater than interval",
			config: CheckerConfig{
				Interval: 5 * time.Second,
				Timeout:  10 * time.Second,
			},
			wantError: true,
		},
		{
			name: "negative healthy threshold",
			config: CheckerConfig{
				Interval:         10 * time.Second,
				Timeout:          3 * time.Second,
				HealthyThreshold: -1,
			},
			wantError: true,
		},
		{
			name: "negative unhealthy threshold",
			config: CheckerConfig{
				Interval:           10 * time.Second,
				Timeout:            3 * time.Second,
				UnhealthyThreshold: -1,
			},
			wantError: true,
		},
		{
			name: "invalid error rate threshold - too low",
			config: CheckerConfig{
				Interval:            10 * time.Second,
				Timeout:             3 * time.Second,
				EnablePassiveChecks: true,
				ErrorRateThreshold:  -0.1,
			},
			wantError: true,
		},
		{
			name: "invalid error rate threshold - too high",
			config: CheckerConfig{
				Interval:            10 * time.Second,
				Timeout:             3 * time.Second,
				EnablePassiveChecks: true,
				ErrorRateThreshold:  1.5,
			},
			wantError: true,
		},
		{
			name: "negative consecutive failures",
			config: CheckerConfig{
				Interval:            10 * time.Second,
				Timeout:             3 * time.Second,
				EnablePassiveChecks: true,
				ConsecutiveFailures: -1,
			},
			wantError: true,
		},
		{
			name: "negative passive check window",
			config: CheckerConfig{
				Interval:            10 * time.Second,
				Timeout:             3 * time.Second,
				EnablePassiveChecks: true,
				PassiveCheckWindow:  -1 * time.Minute,
			},
			wantError: true,
		},
		{
			name: "valid passive check config",
			config: CheckerConfig{
				Interval:            10 * time.Second,
				Timeout:             3 * time.Second,
				EnablePassiveChecks: true,
				ErrorRateThreshold:  0.5,
				ConsecutiveFailures: 5,
				PassiveCheckWindow:  1 * time.Minute,
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.setWithDefaults()
			err := tt.config.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

// TestCheckerConfigDefaults tests that defaults are set correctly
func TestCheckerConfigDefaults(t *testing.T) {
	config := CheckerConfig{}
	config.setWithDefaults()

	if config.Interval != 10*time.Second {
		t.Errorf("Interval = %v, want %v", config.Interval, 10*time.Second)
	}
	if config.Timeout != 3*time.Second {
		t.Errorf("Timeout = %v, want %v", config.Timeout, 3*time.Second)
	}
	if config.HealthyThreshold != 2 {
		t.Errorf("HealthyThreshold = %d, want %d", config.HealthyThreshold, 2)
	}
	if config.UnhealthyThreshold != 3 {
		t.Errorf("UnhealthyThreshold = %d, want %d", config.UnhealthyThreshold, 3)
	}
	if config.ActiveCheckType != CheckTypeTCP {
		t.Errorf("ActiveCheckType = %v, want %v", config.ActiveCheckType, CheckTypeTCP)
	}
	if config.Logger == nil {
		t.Error("Logger should be set to default")
	}
}

// TestNewChecker tests the creation of a new checker
func TestNewChecker(t *testing.T) {
	pool := node.NewPool()
	n1 := node.NewNode("backend1", "localhost:8081", 1)
	n2 := node.NewNode("backend2", "localhost:8082", 1)
	pool.Add(n1)
	pool.Add(n2)

	config := CheckerConfig{
		Interval:           5 * time.Second,
		Timeout:            2 * time.Second,
		HealthyThreshold:   2,
		UnhealthyThreshold: 3,
		ActiveCheckType:    CheckTypeTCP,
	}

	checker, err := NewChecker(pool, config)
	if err != nil {
		t.Fatalf("NewChecker() error = %v", err)
	}

	if checker.interval != 5*time.Second {
		t.Errorf("interval = %v, want %v", checker.interval, 5*time.Second)
	}
	if checker.healthyThreshold != 2 {
		t.Errorf("healthyThreshold = %d, want %d", checker.healthyThreshold, 2)
	}
	if checker.unhealthyThreshold != 3 {
		t.Errorf("unhealthyThreshold = %d, want %d", checker.unhealthyThreshold, 3)
	}
	if len(checker.nh) != 2 {
		t.Errorf("len(nh) = %d, want %d", len(checker.nh), 2)
	}
}

// TestNewCheckerWithInvalidConfig tests that invalid config returns error
func TestNewCheckerWithInvalidConfig(t *testing.T) {
	pool := node.NewPool()

	config := CheckerConfig{
		Interval: -1 * time.Second,
		Timeout:  3 * time.Second,
	}

	_, err := NewChecker(pool, config)
	if err == nil {
		t.Error("NewChecker() should return error for invalid config")
	}
}

// TestNewCheckerWithPassiveChecks tests creation with passive checks enabled
func TestNewCheckerWithPassiveChecks(t *testing.T) {
	pool := node.NewPool()
	n1 := node.NewNode("backend1", "localhost:8081", 1)
	pool.Add(n1)

	config := CheckerConfig{
		Interval:            5 * time.Second,
		Timeout:             2 * time.Second,
		HealthyThreshold:    2,
		UnhealthyThreshold:  3,
		EnablePassiveChecks: true,
		ErrorRateThreshold:  0.5,
		ConsecutiveFailures: 5,
		PassiveCheckWindow:  1 * time.Minute,
	}

	checker, err := NewChecker(pool, config)
	if err != nil {
		t.Fatalf("NewChecker() error = %v", err)
	}

	if checker.passiveChecker == nil {
		t.Error("passiveChecker should be initialized")
	}
}

// TestCheckerStateTransitions tests node state transitions
func TestCheckerStateTransitions(t *testing.T) {
	pool := node.NewPool()
	n1 := node.NewNode("backend1", "localhost:8081", 1)
	pool.Add(n1)

	config := CheckerConfig{
		Interval:           100 * time.Millisecond,
		Timeout:            50 * time.Millisecond,
		HealthyThreshold:   2,
		UnhealthyThreshold: 3,
		Logger:             slog.Default(),
	}

	checker, err := NewChecker(pool, config)
	if err != nil {
		t.Fatalf("NewChecker() error = %v", err)
	}

	// Track state changes
	var stateChanges atomic.Int32
	checker.nh["backend1"].AddListener(func(n *node.Node, old, new NodeState) {
		stateChanges.Add(1)
	})

	// Simulate failures
	for i := 0; i < 3; i++ {
		checker.nh["backend1"].RecordFailure()
	}

	// Check that backend is unhealthy
	state := checker.nh["backend1"].State.Load().(NodeState)
	if state != StateUnhealthy {
		t.Errorf("State = %v, want %v", state, StateUnhealthy)
	}

	// Simulate successes to recover
	for i := 0; i < 2; i++ {
		checker.nh["backend1"].RecordSuccess()
	}

	// Check that backend is healthy again
	state = checker.nh["backend1"].State.Load().(NodeState)
	if state != StateHealthy {
		t.Errorf("State = %v, want %v", state, StateHealthy)
	}

	// Should have 2 state changes (healthy->unhealthy and unhealthy->healthy)
	if stateChanges.Load() != 2 {
		t.Errorf("stateChanges = %d, want %d", stateChanges.Load(), 2)
	}
}

// TestCheckerRecordRequest tests passive health checking through request recording
func TestCheckerRecordRequest(t *testing.T) {
	pool := node.NewPool()
	n1 := node.NewNode("backend1", "localhost:8081", 1)
	pool.Add(n1)

	config := CheckerConfig{
		Interval:            5 * time.Second,
		Timeout:             2 * time.Second,
		HealthyThreshold:    2,
		UnhealthyThreshold:  3,
		EnablePassiveChecks: true,
		ErrorRateThreshold:  0.5,
		ConsecutiveFailures: 3,
		PassiveCheckWindow:  1 * time.Minute,
	}

	checker, err := NewChecker(pool, config)
	if err != nil {
		t.Fatalf("NewChecker() error = %v", err)
	}

	// Record successful requests
	for i := 0; i < 5; i++ {
		checker.RecordRequest(n1, true, 100*time.Millisecond)
	}

	// Verify metrics
	nh := checker.nh["backend1"]
	if nh.m.totalRequests.Load() != 5 {
		t.Errorf("totalRequests = %d, want %d", nh.m.totalRequests.Load(), 5)
	}
	if nh.m.failedRequests.Load() != 0 {
		t.Errorf("failedRequests = %d, want %d", nh.m.failedRequests.Load(), 0)
	}

	// Record failed requests
	for i := 0; i < 3; i++ {
		checker.RecordRequest(n1, false, 100*time.Millisecond)
	}

	if nh.m.totalRequests.Load() != 8 {
		t.Errorf("totalRequests = %d, want %d", nh.m.totalRequests.Load(), 8)
	}
	if nh.m.failedRequests.Load() != 3 {
		t.Errorf("failedRequests = %d, want %d", nh.m.failedRequests.Load(), 3)
	}
}

// TestCheckerRecordRequestWithoutPassiveChecker tests that RecordRequest is no-op when passive checker is disabled
func TestCheckerRecordRequestWithoutPassiveChecker(t *testing.T) {
	pool := node.NewPool()
	n1 := node.NewNode("backend1", "localhost:8081", 1)
	pool.Add(n1)

	config := CheckerConfig{
		Interval:            5 * time.Second,
		Timeout:             2 * time.Second,
		HealthyThreshold:    2,
		UnhealthyThreshold:  3,
		EnablePassiveChecks: false,
	}

	checker, err := NewChecker(pool, config)
	if err != nil {
		t.Fatalf("NewChecker() error = %v", err)
	}

	// This should not panic
	checker.RecordRequest(n1, true, 100*time.Millisecond)

	// Should still record in node health
	nh := checker.nh["backend1"]
	if nh.m.totalRequests.Load() != 1 {
		t.Errorf("totalRequests = %d, want %d", nh.m.totalRequests.Load(), 1)
	}
}

// TestCheckerClose tests cleanup
func TestCheckerClose(t *testing.T) {
	pool := node.NewPool()
	n1 := node.NewNode("backend1", "localhost:8081", 1)
	n2 := node.NewNode("backend2", "localhost:8082", 1)
	pool.Add(n1)
	pool.Add(n2)

	config := CheckerConfig{
		Interval:           5 * time.Second,
		Timeout:            2 * time.Second,
		HealthyThreshold:   2,
		UnhealthyThreshold: 3,
	}

	checker, err := NewChecker(pool, config)
	if err != nil {
		t.Fatalf("NewChecker() error = %v", err)
	}

	if len(checker.nh) != 2 {
		t.Errorf("len(nh) before close = %d, want %d", len(checker.nh), 2)
	}

	err = checker.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	if len(checker.nh) != 0 {
		t.Errorf("len(nh) after close = %d, want %d", len(checker.nh), 0)
	}
}

// TestCheckerStartStop tests starting and stopping the checker
func TestCheckerStartStop(t *testing.T) {
	pool := node.NewPool()
	n1 := node.NewNode("backend1", "localhost:8081", 1)
	pool.Add(n1)

	config := CheckerConfig{
		Interval:           100 * time.Millisecond,
		Timeout:            50 * time.Millisecond,
		HealthyThreshold:   2,
		UnhealthyThreshold: 3,
	}

	checker, err := NewChecker(pool, config)
	if err != nil {
		t.Fatalf("NewChecker() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	checker.Start(ctx)

	// Let it run for a bit
	time.Sleep(250 * time.Millisecond)

	// Stop it
	cancel()

	// Give it time to stop
	time.Sleep(100 * time.Millisecond)

	// Clean up
	err = checker.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

// TestOnStateChange tests the state change callback
func TestOnStateChange(t *testing.T) {
	pool := node.NewPool()
	n1 := node.NewNode("backend1", "localhost:8081", 1)
	pool.Add(n1)

	config := CheckerConfig{
		Interval:            5 * time.Second,
		Timeout:             2 * time.Second,
		HealthyThreshold:    2,
		UnhealthyThreshold:  3,
		EnablePassiveChecks: true,
		ErrorRateThreshold:  0.5,
		ConsecutiveFailures: 5,
		PassiveCheckWindow:  1 * time.Minute,
	}

	checker, err := NewChecker(pool, config)
	if err != nil {
		t.Fatalf("NewChecker() error = %v", err)
	}

	// Track failures
	for i := 0; i < 10; i++ {
		checker.RecordRequest(n1, false, 100*time.Millisecond)
	}

	// Transition to unhealthy should trigger callback
	checker.nh["backend1"].RecordFailure()
	checker.nh["backend1"].RecordFailure()
	checker.nh["backend1"].RecordFailure()

	state := checker.nh["backend1"].State.Load().(NodeState)
	if state != StateUnhealthy {
		t.Errorf("State = %v, want %v", state, StateUnhealthy)
	}

	// Transition back to healthy
	checker.nh["backend1"].RecordSuccess()
	checker.nh["backend1"].RecordSuccess()

	state = checker.nh["backend1"].State.Load().(NodeState)
	if state != StateHealthy {
		t.Errorf("State = %v, want %v", state, StateHealthy)
	}

	// Passive checker should be reset
	tracker := checker.passiveChecker.getTracker(n1)
	if tracker.consecutiveFailures.Load() != 0 {
		t.Errorf("consecutiveFailures after reset = %d, want %d", tracker.consecutiveFailures.Load(), 0)
	}
}

// TestProcessActiveCheckResult tests processing of active check results
func TestProcessActiveCheckResult(t *testing.T) {
	pool := node.NewPool()
	n1 := node.NewNode("backend1", "localhost:8081", 1)
	pool.Add(n1)

	config := CheckerConfig{
		Interval:           5 * time.Second,
		Timeout:            2 * time.Second,
		HealthyThreshold:   2,
		UnhealthyThreshold: 3,
	}

	checker, err := NewChecker(pool, config)
	if err != nil {
		t.Fatalf("NewChecker() error = %v", err)
	}

	// Process successful result
	result := activeCheckResult{
		Backend:    n1,
		Success:    true,
		Duration:   100 * time.Millisecond,
		Timestamp:  time.Now(),
		StatusCode: 200,
	}
	checker.process(result)

	nh := checker.nh["backend1"]
	if nh.m.totalSuccesses.Load() != 1 {
		t.Errorf("totalSuccesses = %d, want %d", nh.m.totalSuccesses.Load(), 1)
	}

	// Process failed result
	result = activeCheckResult{
		Backend:   n1,
		Success:   false,
		Duration:  100 * time.Millisecond,
		Timestamp: time.Now(),
		Error:     context.DeadlineExceeded,
	}
	checker.process(result)

	if nh.m.totalFailures.Load() != 1 {
		t.Errorf("totalFailures = %d, want %d", nh.m.totalFailures.Load(), 1)
	}
}

// TestProcessNewBackend tests that process can handle backends not in the initial pool
func TestProcessNewBackend(t *testing.T) {
	pool := node.NewPool()

	config := CheckerConfig{
		Interval:           5 * time.Second,
		Timeout:            2 * time.Second,
		HealthyThreshold:   2,
		UnhealthyThreshold: 3,
	}

	checker, err := NewChecker(pool, config)
	if err != nil {
		t.Fatalf("NewChecker() error = %v", err)
	}

	if len(checker.nh) != 0 {
		t.Errorf("len(nh) = %d, want %d", len(checker.nh), 0)
	}

	// Process result for new backend
	n1 := node.NewNode("backend1", "localhost:8081", 1)
	result := activeCheckResult{
		Backend:   n1,
		Success:   true,
		Duration:  100 * time.Millisecond,
		Timestamp: time.Now(),
	}
	checker.process(result)

	if len(checker.nh) != 1 {
		t.Errorf("len(nh) = %d, want %d", len(checker.nh), 1)
	}

	nh, exists := checker.nh["backend1"]
	if !exists {
		t.Error("backend1 should be in nh map")
	}
	if nh.m.totalSuccesses.Load() != 1 {
		t.Errorf("totalSuccesses = %d, want %d", nh.m.totalSuccesses.Load(), 1)
	}
}
