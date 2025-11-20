package balance

import "github.com/utkarsh5026/balance/pkg/node"

func checkHealthy(healthy []*node.Node) error {
	if len(healthy) == 0 {
		return ErrNoNodeHealthy
	}
	return nil
}
