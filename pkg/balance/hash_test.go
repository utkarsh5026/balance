package balance

import (
	"fmt"
	"sync"
	"testing"

	"github.com/utkarsh5026/balance/pkg/node"
)

// Helper function to create a pool with test nodes
func createHashTestPool(nodeConfigs []struct{ name, address string; weight int }) *node.Pool {
	pool := node.NewPool()
	for _, config := range nodeConfigs {
		n := node.NewNode(config.name, config.address, config.weight)
		pool.Add(n)
	}
	return pool
}

// TestConsistentHashBalancer_Name tests the Name method
func TestConsistentHashBalancer_Name(t *testing.T) {
	pool := node.NewPool()
	balancer := NewConsistentHashBalancer(pool, 100, "test")

	if balancer.Name() != ConsistentHash {
		t.Errorf("Expected name %s, got %s", ConsistentHash, balancer.Name())
	}
}

// TestConsistentHashBalancer_NewWithDefaults tests constructor with default values
func TestConsistentHashBalancer_NewWithDefaults(t *testing.T) {
	pool := node.NewPool()

	t.Run("default virtual nodes", func(t *testing.T) {
		balancer := NewConsistentHashBalancer(pool, 0, "test")
		if balancer.virtualNodes != defaultVirtualNodes {
			t.Errorf("Expected %d virtual nodes, got %d", defaultVirtualNodes, balancer.virtualNodes)
		}
	})

	t.Run("negative virtual nodes", func(t *testing.T) {
		balancer := NewConsistentHashBalancer(pool, -10, "test")
		if balancer.virtualNodes != defaultVirtualNodes {
			t.Errorf("Expected %d virtual nodes, got %d", defaultVirtualNodes, balancer.virtualNodes)
		}
	})

	t.Run("default hash key", func(t *testing.T) {
		balancer := NewConsistentHashBalancer(pool, 100, "")
		if balancer.hashKey != defaultHashKey {
			t.Errorf("Expected hash key %s, got %s", defaultHashKey, balancer.hashKey)
		}
	})
}

// TestConsistentHashBalancer_SelectNoNodes tests selection with no nodes
func TestConsistentHashBalancer_SelectNoNodes(t *testing.T) {
	pool := node.NewPool()
	balancer := NewConsistentHashBalancer(pool, 100, "test")

	_, err := balancer.Select()
	if err != ErrNoNodeHealthy {
		t.Errorf("Expected ErrNoNodeHealthy, got %v", err)
	}

	_, err = balancer.SelectWithKey("somekey")
	if err != ErrNoNodeHealthy {
		t.Errorf("Expected ErrNoNodeHealthy, got %v", err)
	}
}

// TestConsistentHashBalancer_SelectWithKey tests consistent selection with same key
func TestConsistentHashBalancer_SelectWithKey(t *testing.T) {
	pool := createHashTestPool([]struct{ name, address string; weight int }{
		{"node1", "192.168.1.1:8080", 1},
		{"node2", "192.168.1.2:8080", 1},
		{"node3", "192.168.1.3:8080", 1},
	})

	balancer := NewConsistentHashBalancer(pool, 100, "test")

	t.Run("same key returns same node", func(t *testing.T) {
		key := "user123"
		node1, err := balancer.SelectWithKey(key)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Select 10 times with same key
		for i := 0; i < 10; i++ {
			node2, err := balancer.SelectWithKey(key)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if node1.Address() != node2.Address() {
				t.Errorf("Same key returned different nodes: %s vs %s", node1.Address(), node2.Address())
			}
		}
	})

	t.Run("different keys can return different nodes", func(t *testing.T) {
		nodes := make(map[string]bool)
		// Try many different keys to ensure we hit different nodes
		for i := 0; i < 100; i++ {
			key := fmt.Sprintf("user%d", i)
			node, err := balancer.SelectWithKey(key)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			nodes[node.Address()] = true
		}

		// We should hit at least 2 different nodes with 100 requests
		if len(nodes) < 2 {
			t.Errorf("Expected distribution across multiple nodes, got only %d node(s)", len(nodes))
		}
	})
}

// TestConsistentHashBalancer_Select tests the default Select method
func TestConsistentHashBalancer_Select(t *testing.T) {
	pool := createHashTestPool([]struct{ name, address string; weight int }{
		{"node1", "192.168.1.1:8080", 1},
		{"node2", "192.168.1.2:8080", 1},
		{"node3", "192.168.1.3:8080", 1},
	})

	balancer := NewConsistentHashBalancer(pool, 100, "test")

	t.Run("Select distributes requests", func(t *testing.T) {
		nodes := make(map[string]int)
		requestCount := 300

		for i := 0; i < requestCount; i++ {
			node, err := balancer.SelectWithKey(fmt.Sprintf("req%d", i))
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			nodes[node.Address()]++
		}

		// With 3 nodes and 300 requests, each should get some traffic
		// Allow for variance but ensure all nodes get requests
		if len(nodes) != 3 {
			t.Errorf("Expected 3 nodes to receive requests, got %d", len(nodes))
		}

		for addr, count := range nodes {
			// Each node should get roughly 100 requests (300/3)
			// We'll allow 30-170 range (±70% variance) to account for hash distribution
			if count < 30 || count > 170 {
				t.Logf("Warning: Node %s received %d requests (expected ~100)", addr, count)
			}
		}
	})

	t.Run("Select uses counter for distribution", func(t *testing.T) {
		nodes := make(map[string]bool)
		// Multiple calls to Select() should hit different nodes
		for i := 0; i < 50; i++ {
			node, err := balancer.Select()
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			nodes[node.Address()] = true
		}

		// Should hit at least 2 nodes
		if len(nodes) < 2 {
			t.Errorf("Expected distribution across multiple nodes, got only %d node(s)", len(nodes))
		}
	})
}

// TestConsistentHashBalancer_WeightedDistribution tests weight handling
func TestConsistentHashBalancer_WeightedDistribution(t *testing.T) {
	pool := createHashTestPool([]struct{ name, address string; weight int }{
		{"node1", "192.168.1.1:8080", 1}, // Low weight
		{"node2", "192.168.1.2:8080", 5}, // High weight
		{"node3", "192.168.1.3:8080", 1}, // Low weight
	})

	balancer := NewConsistentHashBalancer(pool, 100, "test")

	nodes := make(map[string]int)
	requestCount := 1000

	for i := 0; i < requestCount; i++ {
		node, err := balancer.SelectWithKey(fmt.Sprintf("key%d", i))
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		nodes[node.Address()]++
	}

	node2Count := nodes["192.168.1.2:8080"]
	node1Count := nodes["192.168.1.1:8080"]
	node3Count := nodes["192.168.1.3:8080"]

	// Node2 has 5x weight, so it should get significantly more requests
	// We expect roughly: node1 ~140, node2 ~720, node3 ~140
	// Allow for variance but node2 should get at least 2x more than others
	if node2Count < node1Count*2 || node2Count < node3Count*2 {
		t.Errorf("Weighted distribution not working: node1=%d, node2=%d, node3=%d",
			node1Count, node2Count, node3Count)
	}

	t.Logf("Distribution - node1: %d, node2: %d, node3: %d", node1Count, node2Count, node3Count)
}

// TestConsistentHashBalancer_NodeAddition tests adding nodes to the pool
func TestConsistentHashBalancer_NodeAddition(t *testing.T) {
	pool := createHashTestPool([]struct{ name, address string; weight int }{
		{"node1", "192.168.1.1:8080", 1},
		{"node2", "192.168.1.2:8080", 1},
	})

	balancer := NewConsistentHashBalancer(pool, 100, "test")

	// Get initial selections for some keys
	key1 := "user123"
	key2 := "user456"
	initialNode1, _ := balancer.SelectWithKey(key1)
	initialNode2, _ := balancer.SelectWithKey(key2)

	// Add a new node
	newNode := node.NewNode("node3", "192.168.1.3:8080", 1)
	pool.Add(newNode)

	// Check selections again
	newNode1, _ := balancer.SelectWithKey(key1)
	newNode2, _ := balancer.SelectWithKey(key2)

	// Some keys might change, some might stay the same
	// This is expected in consistent hashing (minimal disruption)
	changedCount := 0
	if initialNode1.Address() != newNode1.Address() {
		changedCount++
	}
	if initialNode2.Address() != newNode2.Address() {
		changedCount++
	}

	t.Logf("Keys changed after adding node: %d/2", changedCount)

	// Verify the new node is in the ring
	nodes := make(map[string]bool)
	for i := 0; i < 100; i++ {
		n, _ := balancer.SelectWithKey(fmt.Sprintf("key%d", i))
		nodes[n.Address()] = true
	}

	if !nodes["192.168.1.3:8080"] {
		t.Error("New node not found in selection distribution")
	}
}

// TestConsistentHashBalancer_NodeRemoval tests removing nodes from the pool
func TestConsistentHashBalancer_NodeRemoval(t *testing.T) {
	nodes := []*node.Node{
		node.NewNode("node1", "192.168.1.1:8080", 1),
		node.NewNode("node2", "192.168.1.2:8080", 1),
		node.NewNode("node3", "192.168.1.3:8080", 1),
	}

	pool := node.NewPool()
	for _, n := range nodes {
		pool.Add(n)
	}

	balancer := NewConsistentHashBalancer(pool, 100, "test")

	// Mark node2 as unhealthy
	nodes[1].MarkUnhealthy()

	// All selections should now return only node1 or node3
	for i := 0; i < 50; i++ {
		selected, err := balancer.SelectWithKey(fmt.Sprintf("key%d", i))
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if selected.Address() == "192.168.1.2:8080" {
			t.Error("Selected unhealthy node")
		}
	}
}

// TestConsistentHashBalancer_AllNodesUnhealthy tests when all nodes are unhealthy
func TestConsistentHashBalancer_AllNodesUnhealthy(t *testing.T) {
	nodes := []*node.Node{
		node.NewNode("node1", "192.168.1.1:8080", 1),
		node.NewNode("node2", "192.168.1.2:8080", 1),
	}

	pool := node.NewPool()
	for _, n := range nodes {
		pool.Add(n)
	}

	balancer := NewConsistentHashBalancer(pool, 100, "test")

	// Mark all nodes unhealthy
	for _, n := range nodes {
		n.MarkUnhealthy()
	}

	_, err := balancer.Select()
	if err != ErrNoNodeHealthy {
		t.Errorf("Expected ErrNoNodeHealthy, got %v", err)
	}

	_, err = balancer.SelectWithKey("key")
	if err != ErrNoNodeHealthy {
		t.Errorf("Expected ErrNoNodeHealthy, got %v", err)
	}
}

// TestConsistentHashBalancer_VirtualNodeUniqueness tests that virtual nodes have unique hashes
func TestConsistentHashBalancer_VirtualNodeUniqueness(t *testing.T) {
	pool := createHashTestPool([]struct{ name, address string; weight int }{
		{"node1", "192.168.1.1:8080", 1},
		{"node11", "192.168.1.11:8080", 1}, // Similar address to test delimiter
		{"server1", "server1:8080", 1},
		{"server12", "server12:8080", 1}, // Test collision prevention
	})

	balancer := NewConsistentHashBalancer(pool, 50, "test")

	// Check that all ring hashes are unique
	seen := make(map[uint32]bool)
	duplicates := 0

	balancer.mu.RLock()
	for _, hash := range balancer.ring {
		if seen[hash] {
			duplicates++
		}
		seen[hash] = true
	}
	balancer.mu.RUnlock()

	if duplicates > 0 {
		t.Errorf("Found %d duplicate hashes in the ring", duplicates)
	}

	// With 4 nodes, 50 virtual nodes each, we should have 200 positions
	expectedPositions := 4 * 50
	if len(seen) != expectedPositions {
		t.Errorf("Expected %d unique positions, got %d", expectedPositions, len(seen))
	}
}

// TestConsistentHashBalancer_Concurrency tests thread safety
func TestConsistentHashBalancer_Concurrency(t *testing.T) {
	pool := createHashTestPool([]struct{ name, address string; weight int }{
		{"node1", "192.168.1.1:8080", 1},
		{"node2", "192.168.1.2:8080", 1},
		{"node3", "192.168.1.3:8080", 1},
	})

	balancer := NewConsistentHashBalancer(pool, 100, "test")

	// Run concurrent selections
	var wg sync.WaitGroup
	goroutines := 100
	selectionsPerGoroutine := 100

	errors := make(chan error, goroutines*selectionsPerGoroutine)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < selectionsPerGoroutine; j++ {
				key := fmt.Sprintf("goroutine%d-req%d", id, j)
				_, err := balancer.SelectWithKey(key)
				if err != nil {
					errors <- err
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	errorCount := 0
	for err := range errors {
		t.Errorf("Concurrent selection error: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Errorf("Got %d errors during concurrent execution", errorCount)
	}
}

// TestConsistentHashBalancer_ConcurrentPoolModification tests concurrent pool changes
func TestConsistentHashBalancer_ConcurrentPoolModification(t *testing.T) {
	pool := createHashTestPool([]struct{ name, address string; weight int }{
		{"node1", "192.168.1.1:8080", 1},
		{"node2", "192.168.1.2:8080", 1},
	})

	balancer := NewConsistentHashBalancer(pool, 100, "test")

	var wg sync.WaitGroup
	selectionCount := 0
	mu := sync.Mutex{}

	// Goroutine 1: Keep selecting (limited iterations)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			if _, err := balancer.Select(); err == nil {
				mu.Lock()
				selectionCount++
				mu.Unlock()
			}
		}
	}()

	// Goroutine 2: Add and remove nodes
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			n := node.NewNode(fmt.Sprintf("temp%d", i), fmt.Sprintf("192.168.2.%d:8080", i), 1)
			pool.Add(n)
			n.MarkUnhealthy() // Simulate removal by marking unhealthy
		}
	}()

	// Goroutine 3: Toggle health status
	wg.Add(1)
	go func() {
		defer wg.Done()
		healthy := pool.Healthy()
		if len(healthy) > 0 {
			for i := 0; i < 20; i++ {
				healthy[0].MarkUnhealthy()
				healthy[0].MarkHealthy()
			}
		}
	}()

	wg.Wait()

	// If we get here without deadlock or panic, test passes
	t.Logf("Concurrent pool modification test completed successfully with %d selections", selectionCount)
}

// TestConsistentHashBalancer_RingDistribution tests that nodes are well distributed on the ring
func TestConsistentHashBalancer_RingDistribution(t *testing.T) {
	pool := createHashTestPool([]struct{ name, address string; weight int }{
		{"node1", "192.168.1.1:8080", 1},
		{"node2", "192.168.1.2:8080", 1},
		{"node3", "192.168.1.3:8080", 1},
	})

	balancer := NewConsistentHashBalancer(pool, 100, "test")

	balancer.mu.RLock()
	defer balancer.mu.RUnlock()

	// Verify ring is sorted
	for i := 1; i < len(balancer.ring); i++ {
		if balancer.ring[i-1] >= balancer.ring[i] {
			t.Error("Ring is not properly sorted")
			break
		}
	}

	// Verify all ring positions map to nodes
	if len(balancer.ring) != len(balancer.ringMap) {
		t.Errorf("Ring and ringMap size mismatch: ring=%d, map=%d",
			len(balancer.ring), len(balancer.ringMap))
	}

	// Count how many virtual nodes each physical node has
	nodeCounts := make(map[string]int)
	for _, hash := range balancer.ring {
		n := balancer.ringMap[hash]
		nodeCounts[n.Address()]++
	}

	// With equal weights, each node should have approximately the same number
	expectedPerNode := 100 // virtualNodes parameter
	for addr, count := range nodeCounts {
		if count != expectedPerNode {
			t.Errorf("Node %s has %d virtual nodes, expected %d", addr, count, expectedPerNode)
		}
	}
}

// TestConsistentHashBalancer_ZeroWeightNodes tests nodes with zero or negative weight
func TestConsistentHashBalancer_ZeroWeightNodes(t *testing.T) {
	pool := createHashTestPool([]struct{ name, address string; weight int }{
		{"node1", "192.168.1.1:8080", 0},  // Zero weight
		{"node2", "192.168.1.2:8080", -5}, // Negative weight
		{"node3", "192.168.1.3:8080", 1},  // Normal weight
	})

	balancer := NewConsistentHashBalancer(pool, 100, "test")

	// All nodes should still be usable (zero/negative weights are treated as 1)
	nodes := make(map[string]bool)
	for i := 0; i < 100; i++ {
		n, err := balancer.SelectWithKey(fmt.Sprintf("key%d", i))
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		nodes[n.Address()] = true
	}

	// All 3 nodes should be reachable (properWeight normalizes to 1)
	if len(nodes) != 3 {
		t.Errorf("Expected all 3 nodes to be reachable, got %d", len(nodes))
	}
}

// TestConsistentHashBalancer_HashFunction tests the hash function properties
func TestConsistentHashBalancer_HashFunction(t *testing.T) {
	pool := node.NewPool()
	balancer := NewConsistentHashBalancer(pool, 100, "test")

	t.Run("same input produces same hash", func(t *testing.T) {
		key := "testkey"
		hash1 := balancer.hash(key)
		hash2 := balancer.hash(key)

		if hash1 != hash2 {
			t.Error("Hash function is not deterministic")
		}
	})

	t.Run("different inputs produce different hashes", func(t *testing.T) {
		hash1 := balancer.hash("key1")
		hash2 := balancer.hash("key2")

		if hash1 == hash2 {
			t.Error("Different keys produced same hash (unlikely collision)")
		}
	})

	t.Run("empty string is hashable", func(t *testing.T) {
		hash := balancer.hash("")
		if hash == 0 {
			t.Log("Empty string hashes to 0 (may be expected for FNV)")
		}
	})
}
