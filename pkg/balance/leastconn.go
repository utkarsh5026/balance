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

func (l *LeastConnBalancer) Name() string {
	return "Least Connections"
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
