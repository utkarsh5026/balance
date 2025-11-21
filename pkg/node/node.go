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

// NewNode creates a new Node with the given name, address, and weight.
// The node is initialized as healthy.
func NewNode(name, address string, weight int) *Node {
	node := &Node{
		name:    name,
		address: address,
		weight:  weight,
	}
	node.healthy.Store(true)
	return node
}

// Name returns the name of the node.
func (n *Node) Name() string {
	return n.name
}

// Address returns the network address of the node.
func (n *Node) Address() string {
	return n.address
}

// Weight returns the weight of the node used for load balancing.
func (n *Node) Weight() int {
	return n.weight
}

// IsHealthy returns true if the node is currently marked as healthy.
// It is thread-safe.
func (n *Node) IsHealthy() bool {
	return n.healthy.Load()
}

// ActiveConnections returns the current number of active connections to the node.
// It is thread-safe.
func (n *Node) ActiveConnections() int64 {
	return n.activeConnections.Load()
}

// MarkHealthy sets the node's status to healthy.
// It is thread-safe.
func (n *Node) MarkHealthy() {
	n.healthy.Store(true)
}

// MarkUnhealthy sets the node's status to unhealthy.
// It is thread-safe.
func (n *Node) MarkUnhealthy() {
	n.healthy.Store(false)
}

// IncrementActiveConnections increases the active connection count by 1.
// It is thread-safe.
func (n *Node) IncrementActiveConnections() {
	n.activeConnections.Add(1)
}

// DecrementActiveConnections decreases the active connection count by 1.
// It is thread-safe.
func (n *Node) DecrementActiveConnections() {
	n.activeConnections.Add(-1)
}
