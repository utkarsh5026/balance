package node

import (
	"slices"
	"sync"
)

type Pool struct {
	nodes   []*Node
	nodeMap map[string]*Node
	mu      sync.RWMutex
}

func NewPool() *Pool {
	return &Pool{
		nodes:   make([]*Node, 0),
		nodeMap: make(map[string]*Node),
	}
}

func (p *Pool) Add(node *Node) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.nodes = append(p.nodes, node)
	p.nodeMap[node.Name()] = node
}

func (p *Pool) Remove(node *Node) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, n := range p.nodes {
		if n == node {
			p.nodes = append(p.nodes[:i], p.nodes[i+1:]...)
			delete(p.nodeMap, node.Name())
			return true
		}
	}
	return false
}

func (p *Pool) All() []*Node {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return slices.Clone(p.nodes)
}

func (p *Pool) Healthy() []*Node {
	p.mu.RLock()
	defer p.mu.RUnlock()

	healthyNodes := make([]*Node, 0, len(p.nodes))
	for _, n := range p.nodes {
		if n.IsHealthy() {
			healthyNodes = append(healthyNodes, n)
		}
	}

	return healthyNodes
}

func (p *Pool) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.nodes)
}

func (p *Pool) HealthyCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	count := 0
	for _, n := range p.nodes {
		if n.IsHealthy() {
			count++
		}
	}
	return count
}

func (p *Pool) GetByName(name string) *Node {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.nodeMap[name]
}
