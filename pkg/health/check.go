package health

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/utkarsh5026/balance/pkg/node"
)

type CheckerConfig struct {
	// Interval between health checks
	Interval time.Duration

	// Timeout for each health check
	Timeout time.Duration

	// HealthyThreshold is the number of consecutive successes before marking healthy
	HealthyThreshold int

	// UnhealthyThreshold is the number of consecutive failures before marking unhealthy
	UnhealthyThreshold int

	// ActiveCheck configuration
	ActiveCheckType NetworkType
	HTTPPath        string

	// PassiveCheck configuration
	EnablePassiveChecks bool
	ErrorRateThreshold  float64
	ConsecutiveFailures int
	PassiveCheckWindow  time.Duration

	// Logger for health check events (optional, uses slog.Default() if nil)
	Logger *slog.Logger
}

// Validate validates the checker configuration
func (cf *CheckerConfig) Validate() error {
	if cf.Interval < 0 {
		return fmt.Errorf("interval must be positive, got %v", cf.Interval)
	}
	if cf.Timeout < 0 {
		return fmt.Errorf("timeout must be positive, got %v", cf.Timeout)
	}
	if cf.Timeout > cf.Interval {
		return fmt.Errorf("timeout (%v) cannot be greater than interval (%v)", cf.Timeout, cf.Interval)
	}
	if cf.HealthyThreshold < 0 {
		return fmt.Errorf("healthy threshold must be non-negative, got %d", cf.HealthyThreshold)
	}
	if cf.UnhealthyThreshold < 0 {
		return fmt.Errorf("unhealthy threshold must be non-negative, got %d", cf.UnhealthyThreshold)
	}
	if cf.EnablePassiveChecks {
		if cf.ErrorRateThreshold < 0 || cf.ErrorRateThreshold > 1 {
			return fmt.Errorf("error rate threshold must be between 0 and 1, got %f", cf.ErrorRateThreshold)
		}
		if cf.ConsecutiveFailures < 0 {
			return fmt.Errorf("consecutive failures must be non-negative, got %d", cf.ConsecutiveFailures)
		}
		if cf.PassiveCheckWindow < 0 {
			return fmt.Errorf("passive check window must be positive, got %v", cf.PassiveCheckWindow)
		}
	}
	return nil
}

func (cf *CheckerConfig) setWithDefaults() {
	if cf.Interval == 0 {
		cf.Interval = 10 * time.Second
	}
	if cf.Timeout == 0 {
		cf.Timeout = 3 * time.Second
	}
	if cf.HealthyThreshold == 0 {
		cf.HealthyThreshold = 2
	}
	if cf.UnhealthyThreshold == 0 {
		cf.UnhealthyThreshold = 3
	}
	if cf.ActiveCheckType == "" {
		cf.ActiveCheckType = CheckTypeTCP
	}
	if cf.Logger == nil {
		cf.Logger = slog.Default()
	}
}

type Checker struct {
	// Backend pool to monitor
	pool *node.Pool

	// Active health checker
	activeChecker *activeChecker

	// Passive health checker
	passiveChecker *passiveChecker

	// State machines for each backend
	nh map[string]*nodeWithHealth
	mu sync.RWMutex

	interval           time.Duration
	healthyThreshold   int
	unhealthyThreshold int

	logger *slog.Logger
}

func NewChecker(pool *node.Pool, config CheckerConfig) (*Checker, error) {
	config.setWithDefaults()

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	ac := activeCheckConfig{
		NetworkType: config.ActiveCheckType,
		Timeout:     config.Timeout,
		HTTPPath:    config.HTTPPath,
	}

	c := &Checker{
		pool:               pool,
		interval:           config.Interval,
		healthyThreshold:   config.HealthyThreshold,
		unhealthyThreshold: config.UnhealthyThreshold,
		nh:                 make(map[string]*nodeWithHealth),
		activeChecker:      NewActiveHealthChecker(ac),
		logger:             config.Logger,
	}

	if config.EnablePassiveChecks {
		pc := passiveCheckerConfig{
			ErrorRateThreshold:  config.ErrorRateThreshold,
			ConsecutiveFailures: config.ConsecutiveFailures,
			Window:              config.PassiveCheckWindow,
		}
		c.passiveChecker = NewPassiveChecker(pc)
	}

	for _, n := range pool.All() {
		nh := NewNodeWithHealth(n, c.healthyThreshold, c.unhealthyThreshold)
		nh.AddListener(c.onStateChange)
		c.nh[n.Name()] = nh
	}

	return c, nil
}

func (c *Checker) Start(ctx context.Context) {
	c.logger.Info("starting health checker",
		"interval", c.interval,
		"healthy_threshold", c.healthyThreshold,
		"unhealthy_threshold", c.unhealthyThreshold,
	)

	go func() {
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c.performHealthChecks(ctx)
			case <-ctx.Done():
				c.logger.Info("health checker stopped", "reason", ctx.Err())
				return
			}
		}
	}()
}

func (c *Checker) RecordRequest(n *node.Node, success bool, responseTime time.Duration) {
	c.mu.RLock()
	nh, exists := c.nh[n.Name()]
	c.mu.RUnlock()

	if !exists {
		return
	}

	nh.RecordRequest(success, responseTime)
	if c.passiveChecker == nil {
		return
	}

	if success {
		c.passiveChecker.RecordSuccess(n, responseTime)
		return
	}

	nodeHealthy := c.passiveChecker.RecordFailure(n, nil)
	if !nodeHealthy {
		nh.RecordFailure()
	}
}
func (c *Checker) performHealthChecks(ctx context.Context) {
	nodes := c.pool.All()
	if len(nodes) == 0 {
		return
	}

	// Use a timeout shorter than the interval to ensure checks complete before next cycle
	// Reserve 10% of the interval for processing results
	checkTimeout := c.interval * 9 / 10
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	results := c.activeChecker.CheckMultiple(ctx, nodes)

	for _, res := range results {
		c.process(res)
	}
}

func (c *Checker) process(result activeCheckResult) {
	c.mu.RLock()
	nh, exists := c.nh[result.Backend.Name()]
	c.mu.RUnlock()

	if !exists {
		c.mu.Lock()
		nh = NewNodeWithHealth(result.Backend, c.healthyThreshold, c.unhealthyThreshold)
		nh.AddListener(c.onStateChange)
		c.nh[result.Backend.Name()] = nh
		c.mu.Unlock()
	}

	if result.Success {
		c.logger.Debug("health check passed",
			"backend", result.Backend.Name(),
			"duration", result.Duration,
			"status_code", result.StatusCode,
		)
		nh.RecordSuccess()
	} else {
		c.logger.Warn("health check failed",
			"backend", result.Backend.Name(),
			"duration", result.Duration,
			"error", result.Error,
			"status_code", result.StatusCode,
		)
		nh.RecordFailure()
	}
}

func (c *Checker) onStateChange(n *node.Node, oldState, newState NodeState) {
	c.logger.Info("backend state changed",
		"backend", n.Name(),
		"old_state", oldState.String(),
		"new_state", newState.String(),
	)

	if newState == StateHealthy && c.passiveChecker != nil {
		c.passiveChecker.Reset(n)
	}
}

// Close cleans up resources used by the health checker
func (c *Checker) Close() error {
	c.logger.Info("closing health checker")

	if c.activeChecker != nil {
		c.activeChecker.Close()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for name := range c.nh {
		delete(c.nh, name)
	}

	c.logger.Info("health checker closed successfully")
	return nil
}
