package balance

import (
	"errors"

	"github.com/utkarsh5026/balance/pkg/node"
)

var (
	ErrNoNodeHealthy = errors.New("no Node is healthy")
)

type LoadBalancerType string

const (
	RoundRobin               LoadBalancerType = "RoundRobin"
	WeightedRoundRobin       LoadBalancerType = "WeightedRoundRobin"
	LeastConnections         LoadBalancerType = "LeastConnections"
	LeastConnectionsWeighted LoadBalancerType = "LeastConnectionsWeighted"
	SessionAffinity          LoadBalancerType = "SessionAffinity"
)

type LoadBalancer interface {
	// Select selects a backend using the load balancing algorithm
	// Returns nil if no backend is available
	Select() (*node.Node, error)

	// Name returns the name of the load balancing algorithm
	Name() LoadBalancerType
}
