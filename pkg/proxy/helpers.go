package proxy

import (
	"fmt"
	"time"

	"github.com/utkarsh5026/balance/pkg/balance"
	"github.com/utkarsh5026/balance/pkg/conf"
	"github.com/utkarsh5026/balance/pkg/node"
)

const (
	defaultVirtualNodes       = 200
	defaultHashKey            = "source-ip"
	defaultLoadFactor         = 1.25
	defaultSessionTimeout     = 5 * time.Minute
)

func resolveLoadBalancer(cfg *conf.Config, pool *node.Pool) (balance.LoadBalancer, error) {
	var balancer balance.LoadBalancer

	switch cfg.LoadBalancer.Algorithm {
	case "round-robin":
		balancer = balance.NewRoundRobinBalancer(pool)

	case "weighted-round-robin":
		balancer = balance.NewWeightedRoundRobinBalancer(pool)

	case "least-connections":
		balancer = balance.NewLeastConnBalancer(pool)

	case "least-connections-weighted":
		balancer = balance.NewLeastConnWeightedBalancer(pool)

	case "consistent-hash":
		hashKey := cfg.LoadBalancer.HashKey
		if hashKey == "" {
			hashKey = defaultHashKey
		}
		balancer = balance.NewConsistentHashBalancer(pool, defaultVirtualNodes, hashKey)

	case "bounded-consistent-hash":
		hashKey := cfg.LoadBalancer.HashKey
		if hashKey == "" {
			hashKey = defaultHashKey
		}
		balancer = balance.NewBoundedConsistentHashBalancer(pool, defaultVirtualNodes, hashKey, defaultLoadFactor)

	case "session-affinity":
		// Session affinity wraps another load balancer
		// Default to round-robin as the underlying balancer
		baseBalancer := balance.NewRoundRobinBalancer(pool)
		var err error
		balancer, err = balance.NewSessionAffinity(baseBalancer, defaultSessionTimeout)
		if err != nil {
			return nil, fmt.Errorf("failed to create session affinity balancer: %w", err)
		}

	default:
		return nil, fmt.Errorf("unsupported load balancer algorithm: %s. Supported algorithms: round-robin, weighted-round-robin, least-connections, least-connections-weighted, consistent-hash, bounded-consistent-hash, session-affinity", cfg.LoadBalancer.Algorithm)
	}

	return balancer, nil
}

func createNodePool(cfg *conf.Config) *node.Pool {
	pool := node.NewPool()
	for _, nodeCfg := range cfg.Nodes {
		b := node.NewNode(nodeCfg.Name, nodeCfg.Address, nodeCfg.Weight)
		pool.Add(b)
	}
	return pool
}
