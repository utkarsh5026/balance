package node

import (
	"slices"
	"sync"
)

// Pool manages a collection of nodes, providing thread-safe access and modification.
type Pool struct {
	nodes   []*Node
	nodeMap map[string]*Node
	mu      sync.RWMutex
}

// NewPool creates and returns a new, empty Pool instance.
func NewPool() *Pool {
	return &Pool{
		nodes:   make([]*Node, 0),
		nodeMap: make(map[string]*Node),
	}
}

// Add inserts a new node into the pool.
// It is thread-safe.
func (p *Pool) Add(node *Node) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.nodes = append(p.nodes, node)
	p.nodeMap[node.Name()] = node
}

// Remove deletes a node from the pool.
// Returns true if the node was found and removed, false otherwise.
// It is thread-safe.
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

// All returns a slice containing all nodes in the pool.
// The returned slice is a copy, so modifying it won't affect the pool.
// It is thread-safe.
func (p *Pool) All() []*Node {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return slices.Clone(p.nodes)
}

// Healthy returns a slice containing only the healthy nodes in the pool.
// It is thread-safe.
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

// Count returns the total number of nodes in the pool.
// It is thread-safe.
func (p *Pool) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.nodes)
}

// HealthyCount returns the number of healthy nodes in the pool.
// It is thread-safe.
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

// GetByName retrieves a node from the pool by its name.
// Returns nil if no node with the given name exists.
// It is thread-safe.
func (p *Pool) GetByName(name string) *Node {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.nodeMap[name]
}
