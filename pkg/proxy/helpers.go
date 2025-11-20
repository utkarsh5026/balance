package proxy

import (
	"fmt"

	"github.com/utkarsh5026/balance/pkg/balance"
	"github.com/utkarsh5026/balance/pkg/conf"
	"github.com/utkarsh5026/balance/pkg/node"
)

func resolveLoadBalancer(cfg *conf.Config, pool *node.Pool) (balance.LoadBalancer, error) {
	var balancer balance.LoadBalancer
	switch cfg.LoadBalancer.Algorithm {
	case "round-robin":
		balancer = balance.NewRoundRobinBalancer(pool)
	case "least-connections":
		balancer = balance.NewLeastConnBalancer(pool)
	default:
		return nil, fmt.Errorf("unsupported load balancer algorithm: %s", cfg.LoadBalancer.Algorithm)
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
