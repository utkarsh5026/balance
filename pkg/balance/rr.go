package balance

import (
	"slices"
	"sync"
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

// WeightedRoundRobinBalancer implements smooth weighted round-robin algorithm
// as used in NGINX. This provides better distribution than simple weighted round-robin.
//
// Algorithm:
//  1. On each selection, increase current_weight of each node by its weight
//  2. Select the node with the maximum current_weight
//  3. Reduce the selected node's current_weight by the total weight
//
// This ensures smooth distribution. For example, with weights {5, 1, 1}:
// Sequence: a,a,b,a,c,a,a (not a,a,a,a,a,b,c)
type WeightedRoundRobinBalancer struct {
	pool          *node.Pool
	mu            sync.Mutex
	currentWeight map[*node.Node]int
}

func NewWeightedRoundRobinBalancer(pool *node.Pool) *WeightedRoundRobinBalancer {
	return &WeightedRoundRobinBalancer{
		pool:          pool,
		currentWeight: make(map[*node.Node]int),
	}
}

func (b *WeightedRoundRobinBalancer) Name() LoadBalancerType {
	return WeightedRoundRobin
}

func (b *WeightedRoundRobinBalancer) Select() (*node.Node, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	healthy := b.pool.Healthy()
	if err := checkHealthy(healthy); err != nil {
		return nil, err
	}

	if len(healthy) == 1 {
		return healthy[0], nil
	}

	var totalWeight int
	for _, n := range healthy {
		weight := n.Weight()
		if weight <= 0 {
			weight = 1
		}
		totalWeight += weight
	}

	if totalWeight <= 0 {
		totalWeight = len(healthy)
	}

	var selected *node.Node
	maxWeight := -1

	for _, n := range healthy {
		weight := n.Weight()
		if weight <= 0 {
			weight = 1
		}

		b.currentWeight[n] += weight
		if b.currentWeight[n] > maxWeight {
			maxWeight = b.currentWeight[n]
			selected = n
		}
	}

	if selected == nil {
		return healthy[0], nil
	}

	b.currentWeight[selected] -= totalWeight

	for n := range b.currentWeight {
		if !slices.Contains(healthy, n) {
			delete(b.currentWeight, n)
		}
	}

	return selected, nil
}
