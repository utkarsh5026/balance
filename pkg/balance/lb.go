package balance

import (
	"errors"

	"github.com/utkarsh5026/balance/pkg/node"
)

var (
	ErrNoNodeHealthy = errors.New("no Node is healthy")
)

type LoadBalancer interface {
	// Select selects a backend using the load balancing algorithm
	// Returns nil if no backend is available
	Select() (*node.Node, error)

	// Name returns the name of the load balancing algorithm
	Name() string
}
