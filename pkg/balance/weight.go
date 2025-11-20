package balance

import (
	"sync/atomic"

	"github.com/utkarsh5026/balance/pkg/node"
)

type WeightedRoundRobinBalancer struct {
	pool    *node.Pool
	current atomic.Uint64
}

func NewWeightedRoundRobinBalancer(pool *node.Pool) *WeightedRoundRobinBalancer {
	return &WeightedRoundRobinBalancer{
		pool: pool,
	}
}

func (b *WeightedRoundRobinBalancer) Name() string {
	return "WeightedRoundRobin"
}

func (b *WeightedRoundRobinBalancer) Select() (*node.Node, error) {
	healthy := b.pool.Healthy()
	if err := checkHealthy(healthy); err != nil {
		return nil, err
	}

	if len(healthy) == 1 {
		return healthy[0], nil
	}

	var totalWeight int
	for _, n := range healthy {
		totalWeight += n.Weight()
	}

	if totalWeight <= 0 {
		next := b.current.Add(1)
		selected := healthy[int(next)%len(healthy)]
		return selected, nil
	}

	next := b.current.Add(1)
	weightIndex := int(next % uint64(totalWeight))

	for _, n := range healthy {
		weightIndex -= n.Weight()
		if weightIndex < 0 {
			return n, nil
		}
	}

	return healthy[0], nil
}
