package balance

import (
	"sync/atomic"

	"github.com/utkarsh5026/balance/pkg/node"
)

type RoundRobinBalancer struct {
	pool    *node.Pool
	current atomic.Uint64
}

func NewRoundRobinBalancer(pool *node.Pool) *RoundRobinBalancer {
	return &RoundRobinBalancer{
		pool: pool,
	}
}

func (r *RoundRobinBalancer) Name() LoadBalancerType {
	return RoundRobin
}

func (r *RoundRobinBalancer) Select() (*node.Node, error) {
	healthy := r.pool.Healthy()
	if err := checkHealthy(healthy); err != nil {
		return nil, err
	}

	index := r.current.Add(1)
	selected := healthy[int(index)%len(healthy)]
	return selected, nil
}
