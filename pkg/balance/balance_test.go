package balance

import (
	"sync"
	"testing"

	"github.com/utkarsh5026/balance/pkg/node"
)

// Helper function to create a pool with test nodes
func createTestPool(nodeCount int) *node.Pool {
	pool := node.NewPool()
	for i := 1; i <= nodeCount; i++ {
		n := node.NewNode(
			"node"+string(rune('0'+i)),
			"localhost:"+string(rune('0'+8080+i)),
			1,
		)
		pool.Add(n)
	}
	return pool
}

// TestRoundRobinBalancer_Select tests the Round Robin algorithm
func TestRoundRobinBalancer_Select(t *testing.T) {
	t.Run("basic round robin selection", func(t *testing.T) {
		pool := createTestPool(3)
		balancer := NewRoundRobinBalancer(pool)

		// Test that it cycles through nodes
		firstNode, err := balancer.Select()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		secondNode, err := balancer.Select()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		thirdNode, err := balancer.Select()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		fourthNode, err := balancer.Select()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// All nodes should be different for first 3 selections
		if firstNode == secondNode || firstNode == thirdNode || secondNode == thirdNode {
			t.Error("Round robin should select different nodes in sequence")
		}

		// Fourth selection should cycle back to first node
		if firstNode != fourthNode {
			t.Error("Round robin should cycle back to first node")
		}
	})

	t.Run("empty pool", func(t *testing.T) {
		pool := node.NewPool()
		balancer := NewRoundRobinBalancer(pool)

		_, err := balancer.Select()
		if err != ErrNoNodeHealthy {
			t.Errorf("Expected ErrNoNodeHealthy, got %v", err)
		}
	})

	t.Run("all nodes unhealthy", func(t *testing.T) {
		pool := createTestPool(3)
		// Mark all nodes as unhealthy
		for _, n := range pool.All() {
			n.MarkUnhealthy()
		}

		balancer := NewRoundRobinBalancer(pool)
		_, err := balancer.Select()
		if err != ErrNoNodeHealthy {
			t.Errorf("Expected ErrNoNodeHealthy, got %v", err)
		}
	})

	t.Run("some nodes unhealthy", func(t *testing.T) {
		pool := createTestPool(5)
		nodes := pool.All()

		// Mark some nodes as unhealthy
		nodes[1].MarkUnhealthy()
		nodes[3].MarkUnhealthy()

		balancer := NewRoundRobinBalancer(pool)

		// Collect selected nodes
		selectedNodes := make(map[*node.Node]int)
		for i := 0; i < 12; i++ {
			selected, err := balancer.Select()
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			selectedNodes[selected]++
		}

		// Should only select healthy nodes
		if len(selectedNodes) != 3 {
			t.Errorf("Expected 3 healthy nodes to be selected, got %d", len(selectedNodes))
		}

		// Check that unhealthy nodes were not selected
		for n := range selectedNodes {
			if !n.IsHealthy() {
				t.Error("Unhealthy node was selected")
			}
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		pool := createTestPool(5)
		balancer := NewRoundRobinBalancer(pool)

		var wg sync.WaitGroup
		numGoroutines := 100
		selectionsPerGoroutine := 100

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < selectionsPerGoroutine; j++ {
					_, err := balancer.Select()
					if err != nil {
						t.Errorf("Unexpected error: %v", err)
					}
				}
			}()
		}

		wg.Wait()
	})
}

// TestLeastConnBalancer_Select tests the Least Connections algorithm
func TestLeastConnBalancer_Select(t *testing.T) {
	t.Run("selects node with least connections", func(t *testing.T) {
		pool := createTestPool(3)
		nodes := pool.All()

		// Set different connection counts
		nodes[0].IncrementActiveConnections()
		nodes[0].IncrementActiveConnections()
		nodes[0].IncrementActiveConnections() // 3 connections

		nodes[1].IncrementActiveConnections()
		nodes[1].IncrementActiveConnections() // 2 connections

		// nodes[2] has 0 connections

		balancer := NewLeastConnBalancer(pool)
		selected, err := balancer.Select()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Should select node with 0 connections
		if selected != nodes[2] {
			t.Error("Expected to select node with least connections")
		}
	})

	t.Run("empty pool", func(t *testing.T) {
		pool := node.NewPool()
		balancer := NewLeastConnBalancer(pool)

		_, err := balancer.Select()
		if err != ErrNoNodeHealthy {
			t.Errorf("Expected ErrNoNodeHealthy, got %v", err)
		}
	})

	t.Run("all nodes unhealthy", func(t *testing.T) {
		pool := createTestPool(3)
		for _, n := range pool.All() {
			n.MarkUnhealthy()
		}

		balancer := NewLeastConnBalancer(pool)
		_, err := balancer.Select()
		if err != ErrNoNodeHealthy {
			t.Errorf("Expected ErrNoNodeHealthy, got %v", err)
		}
	})

	t.Run("some nodes unhealthy", func(t *testing.T) {
		pool := createTestPool(3)
		nodes := pool.All()

		// Node with least connections but unhealthy
		nodes[0].MarkUnhealthy()

		// Healthy nodes with connections
		nodes[1].IncrementActiveConnections()
		nodes[1].IncrementActiveConnections() // 2 connections

		nodes[2].IncrementActiveConnections() // 1 connection

		balancer := NewLeastConnBalancer(pool)
		selected, err := balancer.Select()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Should select healthy node with least connections
		if selected != nodes[2] {
			t.Error("Expected to select healthy node with least connections")
		}
	})

	t.Run("all nodes have equal connections", func(t *testing.T) {
		pool := createTestPool(3)
		nodes := pool.All()

		// Set equal connections for all nodes
		for _, n := range nodes {
			n.IncrementActiveConnections()
			n.IncrementActiveConnections()
		}

		balancer := NewLeastConnBalancer(pool)
		selected, err := balancer.Select()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Should select first node with minimum connections
		if selected != nodes[0] {
			t.Error("Expected to select first node when all have equal connections")
		}
	})

	t.Run("dynamic connection updates", func(t *testing.T) {
		pool := createTestPool(3)
		nodes := pool.All()
		balancer := NewLeastConnBalancer(pool)

		// Initial state: all nodes have 0 connections
		selected1, err := balancer.Select()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		selected1.IncrementActiveConnections()

		// Second selection should pick a different node
		selected2, err := balancer.Select()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if selected1 == selected2 {
			// If they're the same, it should only happen if it's truly the least connected
			// This is acceptable in the edge case where multiple nodes have 0 connections
			if selected1.ActiveConnections() != 1 {
				t.Error("Expected different node to be selected when connections differ")
			}
		}

		// Increment connections on second selected node
		selected2.IncrementActiveConnections()
		selected2.IncrementActiveConnections()

		// Third selection should pick the node with least connections
		selected3, err := balancer.Select()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Find node with minimum connections
		minConns := int64(^uint64(0) >> 1) // Max int64
		var expectedNode *node.Node
		for _, n := range nodes {
			if n.ActiveConnections() < minConns {
				minConns = n.ActiveConnections()
				expectedNode = n
			}
		}

		if selected3 != expectedNode {
			t.Errorf("Expected node with %d connections, got node with %d connections",
				expectedNode.ActiveConnections(), selected3.ActiveConnections())
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		pool := createTestPool(5)
		balancer := NewLeastConnBalancer(pool)

		var wg sync.WaitGroup
		numGoroutines := 100
		selectionsPerGoroutine := 100

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < selectionsPerGoroutine; j++ {
					_, err := balancer.Select()
					if err != nil {
						t.Errorf("Unexpected error: %v", err)
					}
				}
			}()
		}

		wg.Wait()
	})
}

// TestBalancerName tests the Name() method for both balancers
func TestBalancerName(t *testing.T) {
	pool := createTestPool(1)

	rrBalancer := NewRoundRobinBalancer(pool)
	if rrBalancer.Name() != "Round Robin" {
		t.Errorf("Expected 'Round Robin', got '%s'", rrBalancer.Name())
	}

	lcBalancer := NewLeastConnBalancer(pool)
	if lcBalancer.Name() != "Least Connections" {
		t.Errorf("Expected 'Least Connections', got '%s'", lcBalancer.Name())
	}
}

// TestLoadBalancerInterface ensures both balancers implement the interface
func TestLoadBalancerInterface(t *testing.T) {
	pool := createTestPool(1)

	var _ LoadBalancer = NewRoundRobinBalancer(pool)
	var _ LoadBalancer = NewLeastConnBalancer(pool)
}

// BenchmarkRoundRobinBalancer benchmarks the Round Robin algorithm
func BenchmarkRoundRobinBalancer(b *testing.B) {
	pool := createTestPool(10)
	balancer := NewRoundRobinBalancer(pool)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = balancer.Select()
	}
}

// BenchmarkLeastConnBalancer benchmarks the Least Connections algorithm
func BenchmarkLeastConnBalancer(b *testing.B) {
	pool := createTestPool(10)
	balancer := NewLeastConnBalancer(pool)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = balancer.Select()
	}
}

// BenchmarkRoundRobinConcurrent benchmarks concurrent Round Robin selections
func BenchmarkRoundRobinConcurrent(b *testing.B) {
	pool := createTestPool(10)
	balancer := NewRoundRobinBalancer(pool)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = balancer.Select()
		}
	})
}

// BenchmarkLeastConnConcurrent benchmarks concurrent Least Connections selections
func BenchmarkLeastConnConcurrent(b *testing.B) {
	pool := createTestPool(10)
	balancer := NewLeastConnBalancer(pool)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = balancer.Select()
		}
	})
}
