package health

import (
	"context"
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
}

type Checker struct {
	// Backend pool to monitor
	pool *node.Pool

	// Active health checker
	activeChecker *ActiveHealthChecker

	// Passive health checker
	passiveChecker *passiveChecker

	// State machines for each backend
	nh map[string]*nodeWithHealth
	mu sync.RWMutex

	interval           time.Duration
	healthyThreshold   int
	unhealthyThreshold int
}

func NewChecker(pool *node.Pool, config CheckerConfig) *Checker {
	config.setWithDefaults()
	ac := ActiveCheckerConfig{
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
	}

	if config.EnablePassiveChecks {
		pc := PassiveCheckerConfig{
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

	return c
}

func (c *Checker) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c.performHealthChecks(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (c *Checker) RecordRequest(n *node.Node, success bool, responseTime time.Duration) {
	if c.passiveChecker == nil {
		return
	}

	c.mu.RLock()
	nh, exists := c.nh[n.Name()]
	c.mu.RUnlock()

	if !exists {
		return
	}

	nh.RecordRequest(success, responseTime)

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

	ctx, cancel := context.WithTimeout(ctx, c.interval)
	defer cancel()

	results := c.activeChecker.CheckMultiple(ctx, nodes)

	for _, res := range results {
		c.process(res)
	}
}

func (c *Checker) process(result ActiveCheckResult) {
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
		nh.RecordSuccess()
	} else {
		nh.RecordFailure()
	}
}

func (c *Checker) onStateChange(n *node.Node, oldState, newState NodeState) {
	if newState == StateHealthy && c.passiveChecker != nil {
		c.passiveChecker.Reset(n)
	}
}
