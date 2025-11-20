package balance

import (
	"math"

	"github.com/utkarsh5026/balance/pkg/node"
)

type LeastConnBalancer struct {
	pool *node.Pool
}

func NewLeastConnBalancer(pool *node.Pool) *LeastConnBalancer {
	return &LeastConnBalancer{
		pool: pool,
	}
}

func (l *LeastConnBalancer) Name() LoadBalancerType {
	return LeastConnections
}

func (l *LeastConnBalancer) Select() (*node.Node, error) {
	healthy := l.pool.Healthy()
	if err := checkHealthy(healthy); err != nil {
		return nil, err
	}

	var selected *node.Node
	var leastConns int64 = math.MaxInt64

	for _, n := range healthy {
		activeConns := n.ActiveConnections()
		if activeConns < leastConns {
			leastConns = activeConns
			selected = n
		}
	}
	return selected, nil
}

type LeastConnWeightedBalancer struct {
	pool *node.Pool
}

func NewLeastConnWeightedBalancer(pool *node.Pool) *LeastConnWeightedBalancer {
	return &LeastConnWeightedBalancer{
		pool: pool,
	}
}

func (l *LeastConnWeightedBalancer) Name() LoadBalancerType {
	return LeastConnectionsWeighted
}

func (l *LeastConnWeightedBalancer) Select() (*node.Node, error) {
	healthy := l.pool.Healthy()
	if err := checkHealthy(healthy); err != nil {
		return nil, err
	}

	var selected *node.Node
	var leastWeightedConns float64 = math.MaxFloat64

	for _, n := range healthy {
		activeConns := n.ActiveConnections()
		weight := n.Weight()

		if weight <= 0 {
			weight = 1
		}

		weightedConns := float64(activeConns) / float64(weight)
		if weightedConns < leastWeightedConns {
			leastWeightedConns = weightedConns
			selected = n
		}
	}
	return selected, nil
}
