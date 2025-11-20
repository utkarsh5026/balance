package node

import (
	"sync/atomic"
)

// Node represents a backend server node with health and connection tracking.
// It is safe for concurrent use.
type Node struct {
	name, address     string
	weight            int
	activeConnections atomic.Int64
	healthy           atomic.Bool
}

func NewNode(name, address string, weight int) *Node {
	node := &Node{
		name:    name,
		address: address,
		weight:  weight,
	}
	node.healthy.Store(true)
	return node
}

func (n *Node) Name() string {
	return n.name
}

func (n *Node) Address() string {
	return n.address
}

func (n *Node) Weight() int {
	return n.weight
}

func (n *Node) IsHealthy() bool {
	return n.healthy.Load()
}

func (n *Node) ActiveConnections() int64 {
	return n.activeConnections.Load()
}

func (n *Node) MarkHealthy() {
	n.healthy.Store(true)
}

func (n *Node) MarkUnhealthy() {
	n.healthy.Store(false)
}

func (n *Node) IncrementActiveConnections() {
	n.activeConnections.Add(1)
}

func (n *Node) DecrementActiveConnections() {
	n.activeConnections.Add(-1)
}
