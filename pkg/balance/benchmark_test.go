package balance

import (
	"fmt"
	"testing"
	"time"

	"github.com/utkarsh5026/balance/pkg/node"
)

// setupBenchmarkPool creates a node pool for benchmarking
func setupBenchmarkPool(numNodes int, withWeights bool) *node.Pool {
	pool := node.NewPool()
	for i := 0; i < numNodes; i++ {
		name := fmt.Sprintf("node-%d", i)
		address := fmt.Sprintf("192.168.1.%d:8080", i+1)
		weight := 1
		if withWeights && i%3 == 0 {
			weight = 5
		} else if withWeights && i%3 == 1 {
			weight = 3
		}
		n := node.NewNode(name, address, weight)
		pool.Add(n)
	}
	return pool
}

// BenchmarkRoundRobinScaling benchmarks the round-robin load balancer with different pool sizes
func BenchmarkRoundRobinScaling(b *testing.B) {
	scenarios := []int{5, 10, 50, 100}

	for _, numNodes := range scenarios {
		b.Run(fmt.Sprintf("Nodes_%d", numNodes), func(b *testing.B) {
			pool := setupBenchmarkPool(numNodes, false)
			lb := NewRoundRobinBalancer(pool)

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, err := lb.Select()
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

// BenchmarkWeightedRoundRobinScaling benchmarks the weighted round-robin load balancer
func BenchmarkWeightedRoundRobinScaling(b *testing.B) {
	scenarios := []int{5, 10, 50, 100}

	for _, numNodes := range scenarios {
		b.Run(fmt.Sprintf("Nodes_%d", numNodes), func(b *testing.B) {
			pool := setupBenchmarkPool(numNodes, true)
			lb := NewWeightedRoundRobinBalancer(pool)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := lb.Select()
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkWeightedRoundRobinParallel benchmarks weighted round-robin with concurrent access
func BenchmarkWeightedRoundRobinParallel(b *testing.B) {
	scenarios := []int{5, 10, 50, 100}

	for _, numNodes := range scenarios {
		b.Run(fmt.Sprintf("Nodes_%d", numNodes), func(b *testing.B) {
			pool := setupBenchmarkPool(numNodes, true)
			lb := NewWeightedRoundRobinBalancer(pool)

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, err := lb.Select()
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

// BenchmarkLeastConnScaling benchmarks the least connections load balancer
func BenchmarkLeastConnScaling(b *testing.B) {
	scenarios := []int{5, 10, 50, 100}

	for _, numNodes := range scenarios {
		b.Run(fmt.Sprintf("Nodes_%d", numNodes), func(b *testing.B) {
			pool := setupBenchmarkPool(numNodes, false)
			lb := NewLeastConnBalancer(pool)

			// Simulate varying connection loads
			nodes := pool.All()
			for i, n := range nodes {
				for j := 0; j < i*2; j++ {
					n.IncrementActiveConnections()
				}
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					n, err := lb.Select()
					if err != nil {
						b.Fatal(err)
					}
					n.IncrementActiveConnections()
					n.DecrementActiveConnections()
				}
			})
		})
	}
}

// BenchmarkLeastConnWeightedScaling benchmarks the weighted least connections load balancer
func BenchmarkLeastConnWeightedScaling(b *testing.B) {
	scenarios := []int{5, 10, 50, 100}

	for _, numNodes := range scenarios {
		b.Run(fmt.Sprintf("Nodes_%d", numNodes), func(b *testing.B) {
			pool := setupBenchmarkPool(numNodes, true)
			lb := NewLeastConnWeightedBalancer(pool)

			// Simulate varying connection loads
			nodes := pool.All()
			for i, n := range nodes {
				for j := 0; j < i*3; j++ {
					n.IncrementActiveConnections()
				}
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					n, err := lb.Select()
					if err != nil {
						b.Fatal(err)
					}
					n.IncrementActiveConnections()
					n.DecrementActiveConnections()
				}
			})
		})
	}
}

// BenchmarkConsistentHashScaling benchmarks the consistent hash load balancer
func BenchmarkConsistentHashScaling(b *testing.B) {
	scenarios := []struct {
		nodes        int
		virtualNodes int
	}{
		{5, 100},
		{5, 200},
		{10, 200},
		{50, 200},
		{100, 200},
	}

	for _, scenario := range scenarios {
		b.Run(fmt.Sprintf("Nodes_%d_VNodes_%d", scenario.nodes, scenario.virtualNodes), func(b *testing.B) {
			pool := setupBenchmarkPool(scenario.nodes, false)
			lb := NewConsistentHashBalancer(pool, scenario.virtualNodes, "benchmark-key")

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, err := lb.Select()
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

// BenchmarkConsistentHashWithKey benchmarks consistent hash with key-based selection
func BenchmarkConsistentHashWithKey(b *testing.B) {
	scenarios := []int{5, 10, 50, 100}

	for _, numNodes := range scenarios {
		b.Run(fmt.Sprintf("Nodes_%d", numNodes), func(b *testing.B) {
			pool := setupBenchmarkPool(numNodes, false)
			lb := NewConsistentHashBalancer(pool, 200, "benchmark-key")

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					key := fmt.Sprintf("client-%d", i%1000)
					_, err := lb.SelectWithKey(key)
					if err != nil {
						b.Fatal(err)
					}
					i++
				}
			})
		})
	}
}

// BenchmarkBoundedConsistentHashScaling benchmarks the bounded consistent hash load balancer
func BenchmarkBoundedConsistentHashScaling(b *testing.B) {
	scenarios := []struct {
		nodes      int
		loadFactor float64
	}{
		{5, 1.25},
		{10, 1.25},
		{50, 1.25},
		{100, 1.25},
		{10, 1.5},
		{10, 2.0},
	}

	for _, scenario := range scenarios {
		b.Run(fmt.Sprintf("Nodes_%d_LoadFactor_%.2f", scenario.nodes, scenario.loadFactor), func(b *testing.B) {
			pool := setupBenchmarkPool(scenario.nodes, false)
			lb := NewBoundedConsistentHashBalancer(pool, 200, "benchmark-key", scenario.loadFactor)

			// Simulate varying connection loads
			nodes := pool.All()
			for i, n := range nodes {
				for j := 0; j < i*5; j++ {
					n.IncrementActiveConnections()
				}
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					key := fmt.Sprintf("client-%d", i%1000)
					n, err := lb.SelectWithKey(key)
					if err != nil {
						b.Fatal(err)
					}
					n.IncrementActiveConnections()
					n.DecrementActiveConnections()
					i++
				}
			})
		})
	}
}

// BenchmarkSessionAffinityScaling benchmarks the session affinity load balancer
func BenchmarkSessionAffinityScaling(b *testing.B) {
	scenarios := []struct {
		nodes    int
		sessions int
	}{
		{5, 100},
		{10, 100},
		{10, 1000},
		{50, 1000},
		{100, 1000},
	}

	for _, scenario := range scenarios {
		b.Run(fmt.Sprintf("Nodes_%d_Sessions_%d", scenario.nodes, scenario.sessions), func(b *testing.B) {
			pool := setupBenchmarkPool(scenario.nodes, false)
			baseLB := NewRoundRobinBalancer(pool)
			lb, err := NewSessionAffinity(baseLB, 5*time.Minute)
			if err != nil {
				b.Fatal(err)
			}
			defer lb.Stop()

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					clientIP := fmt.Sprintf("192.168.0.%d", i%scenario.sessions)
					_, err := lb.SelectWithClientIP(clientIP)
					if err != nil {
						b.Fatal(err)
					}
					i++
				}
			})
		})
	}
}

// BenchmarkSessionAffinityNewSessions benchmarks session affinity with new sessions
func BenchmarkSessionAffinityNewSessions(b *testing.B) {
	scenarios := []int{5, 10, 50, 100}

	for _, numNodes := range scenarios {
		b.Run(fmt.Sprintf("Nodes_%d", numNodes), func(b *testing.B) {
			pool := setupBenchmarkPool(numNodes, false)
			baseLB := NewRoundRobinBalancer(pool)
			lb, err := NewSessionAffinity(baseLB, 5*time.Minute)
			if err != nil {
				b.Fatal(err)
			}
			defer lb.Stop()

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					clientIP := fmt.Sprintf("client-%d", i)
					_, err := lb.SelectWithClientIP(clientIP)
					if err != nil {
						b.Fatal(err)
					}
					i++
				}
			})
		})
	}
}

// BenchmarkAllBalancersComparison runs all load balancers with the same configuration for comparison
func BenchmarkAllBalancersComparison(b *testing.B) {
	numNodes := 10

	balancers := []struct {
		name string
		lb   LoadBalancer
	}{
		{"RoundRobin", NewRoundRobinBalancer(setupBenchmarkPool(numNodes, true))},
		{"WeightedRoundRobin", NewWeightedRoundRobinBalancer(setupBenchmarkPool(numNodes, true))},
		{"LeastConn", NewLeastConnBalancer(setupBenchmarkPool(numNodes, false))},
		{"LeastConnWeighted", NewLeastConnWeightedBalancer(setupBenchmarkPool(numNodes, true))},
		{"ConsistentHash", NewConsistentHashBalancer(setupBenchmarkPool(numNodes, false), 200, "benchmark-key")},
	}

	for _, scenario := range balancers {
		b.Run(scenario.name, func(b *testing.B) {
			lb := scenario.lb

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, err := lb.Select()
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

// BenchmarkAllBalancersWithLoad runs all load balancers with connection load simulation
func BenchmarkAllBalancersWithLoad(b *testing.B) {
	numNodes := 10

	balancers := []struct {
		name string
		lb   LoadBalancer
	}{
		{"RoundRobin", NewRoundRobinBalancer(setupBenchmarkPool(numNodes, true))},
		{"WeightedRoundRobin", NewWeightedRoundRobinBalancer(setupBenchmarkPool(numNodes, true))},
		{"LeastConn", NewLeastConnBalancer(setupBenchmarkPool(numNodes, false))},
		{"LeastConnWeighted", NewLeastConnWeightedBalancer(setupBenchmarkPool(numNodes, true))},
		{"ConsistentHash", NewConsistentHashBalancer(setupBenchmarkPool(numNodes, false), 200, "benchmark-key")},
	}

	for _, scenario := range balancers {
		b.Run(scenario.name, func(b *testing.B) {
			lb := scenario.lb

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					n, err := lb.Select()
					if err != nil {
						b.Fatal(err)
					}
					n.IncrementActiveConnections()
					// Simulate some work
					n.DecrementActiveConnections()
				}
			})
		})
	}
}
