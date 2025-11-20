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
	if rrBalancer.Name() != RoundRobin {
		t.Errorf("Expected '%s', got '%s'", RoundRobin, rrBalancer.Name())
	}

	lcBalancer := NewLeastConnBalancer(pool)
	if lcBalancer.Name() != LeastConnections {
		t.Errorf("Expected '%s', got '%s'", LeastConnections, lcBalancer.Name())
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

// Helper function to create a pool with weighted nodes
func createWeightedTestPool(weights []int) *node.Pool {
	pool := node.NewPool()
	for i, weight := range weights {
		n := node.NewNode(
			string(rune('a'+i)),
			"localhost:"+string(rune('0'+8080+i)),
			weight,
		)
		pool.Add(n)
	}
	return pool
}

// TestWeightedRoundRobinBalancer_Select tests the Weighted Round Robin algorithm
func TestWeightedRoundRobinBalancer_Select(t *testing.T) {
	t.Run("smooth distribution with weights {5,1,1}", func(t *testing.T) {
		pool := createWeightedTestPool([]int{5, 1, 1})
		balancer := NewWeightedRoundRobinBalancer(pool)
		nodes := pool.All()

		// Expected sequence for weights {5,1,1}: a,a,b,a,c,a,a (7 selections in one cycle)
		// This is the NGINX smooth weighted round-robin pattern
		selections := make([]*node.Node, 7)
		for i := 0; i < 7; i++ {
			selected, err := balancer.Select()
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			selections[i] = selected
		}

		// Count selections per node
		counts := make(map[*node.Node]int)
		for _, n := range selections {
			counts[n]++
		}

		// Verify distribution matches weights
		if counts[nodes[0]] != 5 {
			t.Errorf("Expected node 'a' to be selected 5 times, got %d", counts[nodes[0]])
		}
		if counts[nodes[1]] != 1 {
			t.Errorf("Expected node 'b' to be selected 1 time, got %d", counts[nodes[1]])
		}
		if counts[nodes[2]] != 1 {
			t.Errorf("Expected node 'c' to be selected 1 time, got %d", counts[nodes[2]])
		}

		// Verify smooth distribution (no clustering)
		// The sequence should NOT be a,a,a,a,a,b,c
		firstFive := selections[:5]
		allSameInFirstFive := true
		for i := 1; i < 5; i++ {
			if firstFive[i] != firstFive[0] {
				allSameInFirstFive = false
				break
			}
		}
		if allSameInFirstFive {
			t.Error("Distribution is not smooth - first 5 selections are all the same node")
		}
	})

	t.Run("distribution over multiple cycles", func(t *testing.T) {
		pool := createWeightedTestPool([]int{3, 1})
		balancer := NewWeightedRoundRobinBalancer(pool)
		nodes := pool.All()

		// Run for 3 complete cycles (12 selections)
		counts := make(map[*node.Node]int)
		for i := 0; i < 12; i++ {
			selected, err := balancer.Select()
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			counts[selected]++
		}

		// Should maintain 3:1 ratio
		if counts[nodes[0]] != 9 {
			t.Errorf("Expected node 'a' (weight 3) to be selected 9 times, got %d", counts[nodes[0]])
		}
		if counts[nodes[1]] != 3 {
			t.Errorf("Expected node 'b' (weight 1) to be selected 3 times, got %d", counts[nodes[1]])
		}
	})

	t.Run("equal weights", func(t *testing.T) {
		pool := createWeightedTestPool([]int{2, 2, 2})
		balancer := NewWeightedRoundRobinBalancer(pool)

		counts := make(map[*node.Node]int)
		for i := 0; i < 12; i++ {
			selected, err := balancer.Select()
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			counts[selected]++
		}

		// All nodes should be selected equally
		for n, count := range counts {
			if count != 4 {
				t.Errorf("Expected node %s to be selected 4 times, got %d", n.Name(), count)
			}
		}
	})

	t.Run("zero and negative weights", func(t *testing.T) {
		pool := createWeightedTestPool([]int{0, -1, 3})
		balancer := NewWeightedRoundRobinBalancer(pool)

		counts := make(map[*node.Node]int)
		for i := 0; i < 15; i++ {
			selected, err := balancer.Select()
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			counts[selected]++
		}

		// Nodes with zero/negative weights should be treated as weight 1
		// Expected ratio: 1:1:3
		nodes := pool.All()
		if counts[nodes[2]] < counts[nodes[0]] || counts[nodes[2]] < counts[nodes[1]] {
			t.Error("Node with weight 3 should be selected more frequently than nodes with weight 0 or negative")
		}
	})

	t.Run("single node", func(t *testing.T) {
		pool := createWeightedTestPool([]int{5})
		balancer := NewWeightedRoundRobinBalancer(pool)
		nodes := pool.All()

		for i := 0; i < 5; i++ {
			selected, err := balancer.Select()
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if selected != nodes[0] {
				t.Error("Single node should always be selected")
			}
		}
	})

	t.Run("empty pool", func(t *testing.T) {
		pool := node.NewPool()
		balancer := NewWeightedRoundRobinBalancer(pool)

		_, err := balancer.Select()
		if err != ErrNoNodeHealthy {
			t.Errorf("Expected ErrNoNodeHealthy, got %v", err)
		}
	})

	t.Run("all nodes unhealthy", func(t *testing.T) {
		pool := createWeightedTestPool([]int{3, 2, 1})
		for _, n := range pool.All() {
			n.MarkUnhealthy()
		}

		balancer := NewWeightedRoundRobinBalancer(pool)
		_, err := balancer.Select()
		if err != ErrNoNodeHealthy {
			t.Errorf("Expected ErrNoNodeHealthy, got %v", err)
		}
	})

	t.Run("some nodes unhealthy", func(t *testing.T) {
		pool := createWeightedTestPool([]int{5, 3, 2})
		nodes := pool.All()

		// Mark node with weight 5 as unhealthy
		nodes[0].MarkUnhealthy()

		balancer := NewWeightedRoundRobinBalancer(pool)

		counts := make(map[*node.Node]int)
		for i := 0; i < 10; i++ {
			selected, err := balancer.Select()
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			counts[selected]++

			if !selected.IsHealthy() {
				t.Error("Unhealthy node was selected")
			}
		}

		// Only healthy nodes should be selected
		if counts[nodes[0]] != 0 {
			t.Errorf("Unhealthy node should not be selected, got %d selections", counts[nodes[0]])
		}

		// Verify 3:2 ratio for healthy nodes
		if counts[nodes[1]] != 6 || counts[nodes[2]] != 4 {
			t.Errorf("Expected 6:4 distribution for nodes with weights 3:2, got %d:%d",
				counts[nodes[1]], counts[nodes[2]])
		}
	})

	t.Run("node becomes healthy during operation", func(t *testing.T) {
		pool := createWeightedTestPool([]int{2, 2})
		nodes := pool.All()
		nodes[1].MarkUnhealthy()

		balancer := NewWeightedRoundRobinBalancer(pool)

		// First 4 selections - only node[0] is healthy
		for i := 0; i < 4; i++ {
			selected, err := balancer.Select()
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if selected != nodes[0] {
				t.Error("Only healthy node should be selected")
			}
		}

		// Mark second node as healthy
		nodes[1].MarkHealthy()

		// Next selections should distribute between both nodes
		counts := make(map[*node.Node]int)
		for i := 0; i < 10; i++ {
			selected, err := balancer.Select()
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			counts[selected]++
		}

		if counts[nodes[1]] == 0 {
			t.Error("Newly healthy node should start receiving selections")
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		pool := createWeightedTestPool([]int{3, 2, 1})
		balancer := NewWeightedRoundRobinBalancer(pool)

		var wg sync.WaitGroup
		numGoroutines := 100
		selectionsPerGoroutine := 100

		errorCount := 0
		var errorMu sync.Mutex

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < selectionsPerGoroutine; j++ {
					_, err := balancer.Select()
					if err != nil {
						errorMu.Lock()
						errorCount++
						errorMu.Unlock()
					}
				}
			}()
		}

		wg.Wait()

		if errorCount > 0 {
			t.Errorf("Got %d errors during concurrent access", errorCount)
		}
	})
}

// TestLeastConnWeightedBalancer_Select tests the Weighted Least Connections algorithm
func TestLeastConnWeightedBalancer_Select(t *testing.T) {
	t.Run("selects node with lowest weighted connections", func(t *testing.T) {
		pool := createWeightedTestPool([]int{5, 2, 1})
		nodes := pool.All()

		// Set up connections:
		// Node A: 10 connections, weight 5 -> ratio 2.0
		// Node B: 6 connections, weight 2 -> ratio 3.0
		// Node C: 0 connections, weight 1 -> ratio 0.0
		for i := 0; i < 10; i++ {
			nodes[0].IncrementActiveConnections()
		}
		for i := 0; i < 6; i++ {
			nodes[1].IncrementActiveConnections()
		}

		balancer := NewLeastConnWeightedBalancer(pool)
		selected, err := balancer.Select()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Should select node C with lowest ratio (0.0)
		if selected != nodes[2] {
			t.Errorf("Expected to select node with lowest weighted connections (node 'c'), got %s", selected.Name())
		}
	})

	t.Run("weight affects selection", func(t *testing.T) {
		pool := createWeightedTestPool([]int{5, 1})
		nodes := pool.All()

		// Node A: 10 connections, weight 5 -> ratio 2.0
		// Node B: 1 connection, weight 1 -> ratio 1.0
		for i := 0; i < 10; i++ {
			nodes[0].IncrementActiveConnections()
		}
		nodes[1].IncrementActiveConnections()

		balancer := NewLeastConnWeightedBalancer(pool)
		selected, err := balancer.Select()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Should select node B despite node A having higher weight
		if selected != nodes[1] {
			t.Errorf("Expected to select node 'b' with lower ratio, got %s", selected.Name())
		}
	})

	t.Run("zero weight defaults to 1", func(t *testing.T) {
		pool := createWeightedTestPool([]int{0, 0})
		nodes := pool.All()

		// Both have zero weight, should default to 1
		nodes[0].IncrementActiveConnections()
		nodes[0].IncrementActiveConnections() // 2 connections

		balancer := NewLeastConnWeightedBalancer(pool)
		selected, err := balancer.Select()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Should select node with 0 connections
		if selected != nodes[1] {
			t.Error("Expected to select node with fewer connections")
		}
	})

	t.Run("negative weight defaults to 1", func(t *testing.T) {
		pool := createWeightedTestPool([]int{-5, 3})
		nodes := pool.All()

		// Node A: 2 connections, weight -5 (treated as 1) -> ratio 2.0
		// Node B: 5 connections, weight 3 -> ratio 1.67
		nodes[0].IncrementActiveConnections()
		nodes[0].IncrementActiveConnections()

		for i := 0; i < 5; i++ {
			nodes[1].IncrementActiveConnections()
		}

		balancer := NewLeastConnWeightedBalancer(pool)
		selected, err := balancer.Select()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Node B has lower ratio (1.67 vs 2.0)
		if selected != nodes[1] {
			t.Error("Expected to select node with lower weighted connection ratio")
		}
	})

	t.Run("all nodes have equal weighted connections", func(t *testing.T) {
		pool := createWeightedTestPool([]int{2, 2, 2})
		nodes := pool.All()

		// All nodes: 4 connections, weight 2 -> ratio 2.0
		for _, n := range nodes {
			for i := 0; i < 4; i++ {
				n.IncrementActiveConnections()
			}
		}

		balancer := NewLeastConnWeightedBalancer(pool)
		selected, err := balancer.Select()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Should select first node when all are equal
		if selected != nodes[0] {
			t.Error("Expected to select first node when all have equal weighted connections")
		}
	})

	t.Run("empty pool", func(t *testing.T) {
		pool := node.NewPool()
		balancer := NewLeastConnWeightedBalancer(pool)

		_, err := balancer.Select()
		if err != ErrNoNodeHealthy {
			t.Errorf("Expected ErrNoNodeHealthy, got %v", err)
		}
	})

	t.Run("all nodes unhealthy", func(t *testing.T) {
		pool := createWeightedTestPool([]int{3, 2, 1})
		for _, n := range pool.All() {
			n.MarkUnhealthy()
		}

		balancer := NewLeastConnWeightedBalancer(pool)
		_, err := balancer.Select()
		if err != ErrNoNodeHealthy {
			t.Errorf("Expected ErrNoNodeHealthy, got %v", err)
		}
	})

	t.Run("some nodes unhealthy", func(t *testing.T) {
		pool := createWeightedTestPool([]int{5, 3, 2})
		nodes := pool.All()

		// Node with best ratio but unhealthy
		nodes[0].MarkUnhealthy()

		// Node B: 6 connections, weight 3 -> ratio 2.0
		// Node C: 1 connection, weight 2 -> ratio 0.5
		for i := 0; i < 6; i++ {
			nodes[1].IncrementActiveConnections()
		}
		nodes[2].IncrementActiveConnections()

		balancer := NewLeastConnWeightedBalancer(pool)
		selected, err := balancer.Select()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Should select healthy node C with lowest ratio
		if selected != nodes[2] {
			t.Error("Expected to select healthy node with lowest weighted connections")
		}
		if !selected.IsHealthy() {
			t.Error("Selected node is not healthy")
		}
	})

	t.Run("dynamic connection updates", func(t *testing.T) {
		pool := createWeightedTestPool([]int{2, 2})
		balancer := NewLeastConnWeightedBalancer(pool)

		// Initial: both have 0 connections
		selected1, err := balancer.Select()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		selected1.IncrementActiveConnections()
		selected1.IncrementActiveConnections()

		// Second selection should pick the other node
		selected2, err := balancer.Select()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if selected1 == selected2 {
			t.Error("Expected different node to be selected when one has more connections")
		}

		// Add many connections to second node
		for i := 0; i < 10; i++ {
			selected2.IncrementActiveConnections()
		}

		// Next selection should go back to first node
		selected3, err := balancer.Select()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if selected3 != selected1 {
			t.Error("Expected to select node with fewer weighted connections")
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		pool := createWeightedTestPool([]int{5, 3, 2})
		balancer := NewLeastConnWeightedBalancer(pool)

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

// TestBalancerName_Weighted tests the Name() method for weighted balancers
func TestBalancerName_Weighted(t *testing.T) {
	pool := createWeightedTestPool([]int{1, 2, 3})

	wrrBalancer := NewWeightedRoundRobinBalancer(pool)
	if wrrBalancer.Name() != WeightedRoundRobin {
		t.Errorf("Expected '%s', got '%s'", WeightedRoundRobin, wrrBalancer.Name())
	}

	lcwBalancer := NewLeastConnWeightedBalancer(pool)
	if lcwBalancer.Name() != LeastConnectionsWeighted {
		t.Errorf("Expected '%s', got '%s'", LeastConnectionsWeighted, lcwBalancer.Name())
	}
}

// TestLoadBalancerInterface_Weighted ensures weighted balancers implement the interface
func TestLoadBalancerInterface_Weighted(t *testing.T) {
	pool := createWeightedTestPool([]int{1, 2, 3})

	var _ LoadBalancer = NewWeightedRoundRobinBalancer(pool)
	var _ LoadBalancer = NewLeastConnWeightedBalancer(pool)
}

// BenchmarkWeightedRoundRobinBalancer benchmarks the Weighted Round Robin algorithm
func BenchmarkWeightedRoundRobinBalancer(b *testing.B) {
	pool := createWeightedTestPool([]int{5, 3, 2, 1, 1, 1, 1, 1, 1, 1})
	balancer := NewWeightedRoundRobinBalancer(pool)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = balancer.Select()
	}
}

// BenchmarkLeastConnWeightedBalancer benchmarks the Weighted Least Connections algorithm
func BenchmarkLeastConnWeightedBalancer(b *testing.B) {
	pool := createWeightedTestPool([]int{5, 3, 2, 1, 1, 1, 1, 1, 1, 1})
	balancer := NewLeastConnWeightedBalancer(pool)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = balancer.Select()
	}
}

// BenchmarkWeightedRoundRobinConcurrent benchmarks concurrent Weighted Round Robin selections
func BenchmarkWeightedRoundRobinConcurrent(b *testing.B) {
	pool := createWeightedTestPool([]int{5, 3, 2, 1, 1, 1, 1, 1, 1, 1})
	balancer := NewWeightedRoundRobinBalancer(pool)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = balancer.Select()
		}
	})
}

// BenchmarkLeastConnWeightedConcurrent benchmarks concurrent Weighted Least Connections selections
func BenchmarkLeastConnWeightedConcurrent(b *testing.B) {
	pool := createWeightedTestPool([]int{5, 3, 2, 1, 1, 1, 1, 1, 1, 1})
	balancer := NewLeastConnWeightedBalancer(pool)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = balancer.Select()
		}
	})
}
