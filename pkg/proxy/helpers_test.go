package proxy

import (
	"testing"

	"github.com/utkarsh5026/balance/pkg/conf"
	"github.com/utkarsh5026/balance/pkg/node"
)

func TestResolveLoadBalancer(t *testing.T) {
	tests := []struct {
		name      string
		algorithm string
		hashKey   string
		wantErr   bool
	}{
		{
			name:      "round-robin",
			algorithm: "round-robin",
			wantErr:   false,
		},
		{
			name:      "weighted-round-robin",
			algorithm: "weighted-round-robin",
			wantErr:   false,
		},
		{
			name:      "least-connections",
			algorithm: "least-connections",
			wantErr:   false,
		},
		{
			name:      "least-connections-weighted",
			algorithm: "least-connections-weighted",
			wantErr:   false,
		},
		{
			name:      "consistent-hash",
			algorithm: "consistent-hash",
			hashKey:   "source-ip",
			wantErr:   false,
		},
		{
			name:      "consistent-hash-default-key",
			algorithm: "consistent-hash",
			wantErr:   false,
		},
		{
			name:      "bounded-consistent-hash",
			algorithm: "bounded-consistent-hash",
			hashKey:   "source-ip",
			wantErr:   false,
		},
		{
			name:      "session-affinity",
			algorithm: "session-affinity",
			wantErr:   false,
		},
		{
			name:      "invalid-algorithm",
			algorithm: "invalid-algorithm",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := node.NewPool()
			// Add some test nodes
			for i := 1; i <= 3; i++ {
				n := node.NewNode(
					"test-node",
					"localhost:9000",
					1,
				)
				pool.Add(n)
			}

			cfg := &conf.Config{
				LoadBalancer: conf.LoadBalancerConfig{
					Algorithm: tt.algorithm,
					HashKey:   tt.hashKey,
				},
			}

			balancer, err := resolveLoadBalancer(cfg, pool)
			if tt.wantErr {
				if err == nil {
					t.Errorf("resolveLoadBalancer() expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("resolveLoadBalancer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if balancer == nil {
				t.Errorf("resolveLoadBalancer() returned nil balancer")
			}

			// Test that we can actually select a node
			selectedNode, err := balancer.Select()
			if err != nil {
				t.Errorf("balancer.Select() error = %v", err)
			}
			if selectedNode == nil {
				t.Errorf("balancer.Select() returned nil node")
			}
		})
	}
}

func TestCreateNodePool(t *testing.T) {
	cfg := &conf.Config{
		Nodes: []conf.Node{
			{
				Name:    "backend-1",
				Address: "localhost:9001",
				Weight:  1,
			},
			{
				Name:    "backend-2",
				Address: "localhost:9002",
				Weight:  2,
			},
			{
				Name:    "backend-3",
				Address: "localhost:9003",
				Weight:  3,
			},
		},
	}

	pool := createNodePool(cfg)
	if pool == nil {
		t.Fatal("createNodePool() returned nil")
	}

	nodes := pool.All()
	if len(nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(nodes))
	}

	// Verify nodes were created correctly
	expectedAddresses := map[string]int{
		"localhost:9001": 1,
		"localhost:9002": 2,
		"localhost:9003": 3,
	}

	for _, n := range nodes {
		expectedWeight, exists := expectedAddresses[n.Address()]
		if !exists {
			t.Errorf("unexpected node address: %s", n.Address())
		}
		if n.Weight() != expectedWeight {
			t.Errorf("node %s has weight %d, expected %d", n.Address(), n.Weight(), expectedWeight)
		}
		if !n.IsHealthy() {
			t.Errorf("node %s should be healthy", n.Address())
		}
	}
}

func BenchmarkResolveLoadBalancer(b *testing.B) {
	algorithms := []string{
		"round-robin",
		"weighted-round-robin",
		"least-connections",
		"least-connections-weighted",
		"consistent-hash",
		"bounded-consistent-hash",
		"session-affinity",
	}

	pool := node.NewPool()
	for i := 1; i <= 10; i++ {
		n := node.NewNode("test-node", "localhost:9000", 1)
		pool.Add(n)
	}

	for _, algo := range algorithms {
		b.Run(algo, func(b *testing.B) {
			cfg := &conf.Config{
				LoadBalancer: conf.LoadBalancerConfig{
					Algorithm: algo,
					HashKey:   "source-ip",
				},
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := resolveLoadBalancer(cfg, pool)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
