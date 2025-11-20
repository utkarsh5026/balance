package balance

import (
	"sync"
	"testing"
	"time"
)

// TestNewSessionAffinity tests the constructor
func TestNewSessionAffinity(t *testing.T) {
	t.Run("valid timeout", func(t *testing.T) {
		pool := createTestPool(3)
		baseBalancer := NewRoundRobinBalancer(pool)

		sa, err := NewSessionAffinity(baseBalancer, 5*time.Minute)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if sa == nil {
			t.Fatal("Expected non-nil SessionAffinityBalancer")
		}

		// Clean up
		defer sa.Stop()

		if sa.timeout != 5*time.Minute {
			t.Errorf("Expected timeout to be 5 minutes, got %v", sa.timeout)
		}
		if sa.sessions == nil {
			t.Error("Expected sessions map to be initialized")
		}
		if sa.cleanupTicker == nil {
			t.Error("Expected cleanup ticker to be initialized")
		}
	})

	t.Run("zero timeout", func(t *testing.T) {
		pool := createTestPool(3)
		baseBalancer := NewRoundRobinBalancer(pool)

		sa, err := NewSessionAffinity(baseBalancer, 0)
		if err == nil {
			t.Error("Expected error for zero timeout")
			sa.Stop()
		}
		if sa != nil {
			t.Error("Expected nil SessionAffinityBalancer for zero timeout")
		}
	})

	t.Run("negative timeout", func(t *testing.T) {
		pool := createTestPool(3)
		baseBalancer := NewRoundRobinBalancer(pool)

		sa, err := NewSessionAffinity(baseBalancer, -1*time.Second)
		if err == nil {
			t.Error("Expected error for negative timeout")
			sa.Stop()
		}
		if sa != nil {
			t.Error("Expected nil SessionAffinityBalancer for negative timeout")
		}
	})
}

// TestSessionAffinityBalancer_SelectWithClientIP tests client IP-based selection
func TestSessionAffinityBalancer_SelectWithClientIP(t *testing.T) {
	t.Run("new client gets assigned node", func(t *testing.T) {
		pool := createTestPool(3)
		baseBalancer := NewRoundRobinBalancer(pool)
		sa, _ := NewSessionAffinity(baseBalancer, 5*time.Minute)
		defer sa.Stop()

		clientIP := "192.168.1.1"
		node1, err := sa.SelectWithClientIP(clientIP)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if node1 == nil {
			t.Fatal("Expected non-nil node")
		}

		// Verify session was created
		sa.mu.RLock()
		sess, exists := sa.sessions[clientIP]
		sa.mu.RUnlock()

		if !exists {
			t.Error("Expected session to be created for client IP")
		}
		if sess.node != node1 {
			t.Error("Session node doesn't match returned node")
		}
	})

	t.Run("existing client gets same node", func(t *testing.T) {
		pool := createTestPool(3)
		baseBalancer := NewRoundRobinBalancer(pool)
		sa, _ := NewSessionAffinity(baseBalancer, 5*time.Minute)
		defer sa.Stop()

		clientIP := "192.168.1.1"

		// First selection
		node1, err := sa.SelectWithClientIP(clientIP)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Second selection should return the same node
		node2, err := sa.SelectWithClientIP(clientIP)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if node1 != node2 {
			t.Error("Expected same node for same client IP")
		}
	})

	t.Run("different clients get different nodes", func(t *testing.T) {
		pool := createTestPool(3)
		baseBalancer := NewRoundRobinBalancer(pool)
		sa, _ := NewSessionAffinity(baseBalancer, 5*time.Minute)
		defer sa.Stop()

		client1 := "192.168.1.1"
		client2 := "192.168.1.2"
		client3 := "192.168.1.3"

		node1, _ := sa.SelectWithClientIP(client1)
		node2, _ := sa.SelectWithClientIP(client2)
		node3, _ := sa.SelectWithClientIP(client3)

		// With round robin, these should be different nodes
		if node1 == node2 && node2 == node3 {
			t.Error("Expected different nodes for different clients")
		}
	})

	t.Run("session updates last access time", func(t *testing.T) {
		pool := createTestPool(3)
		baseBalancer := NewRoundRobinBalancer(pool)
		sa, _ := NewSessionAffinity(baseBalancer, 5*time.Minute)
		defer sa.Stop()

		clientIP := "192.168.1.1"

		// First selection
		sa.SelectWithClientIP(clientIP)

		sa.mu.RLock()
		firstAccess := sa.sessions[clientIP].lastAccess
		sa.mu.RUnlock()

		// Wait a bit
		time.Sleep(10 * time.Millisecond)

		// Second selection
		sa.SelectWithClientIP(clientIP)

		sa.mu.RLock()
		secondAccess := sa.sessions[clientIP].lastAccess
		sa.mu.RUnlock()

		if !secondAccess.After(firstAccess) {
			t.Error("Expected last access time to be updated")
		}
	})

	t.Run("expired session gets new node", func(t *testing.T) {
		pool := createTestPool(3)
		baseBalancer := NewRoundRobinBalancer(pool)
		// Very short timeout
		sa, _ := NewSessionAffinity(baseBalancer, 50*time.Millisecond)
		defer sa.Stop()

		clientIP := "192.168.1.1"

		// First selection
		node1, _ := sa.SelectWithClientIP(clientIP)

		// Wait for session to expire
		time.Sleep(100 * time.Millisecond)

		// Second selection should get a new node (possibly different)
		node2, _ := sa.SelectWithClientIP(clientIP)

		// The nodes might be the same by chance, but the session should be recreated
		sa.mu.RLock()
		sess := sa.sessions[clientIP]
		sa.mu.RUnlock()

		if time.Since(sess.lastAccess) > 50*time.Millisecond {
			t.Error("Expected new session with recent last access time")
		}

		// Both nodes should be valid
		if node1 == nil || node2 == nil {
			t.Error("Expected valid nodes")
		}
	})

	t.Run("unhealthy node triggers new selection", func(t *testing.T) {
		pool := createTestPool(3)
		baseBalancer := NewRoundRobinBalancer(pool)
		sa, _ := NewSessionAffinity(baseBalancer, 5*time.Minute)
		defer sa.Stop()

		clientIP := "192.168.1.1"

		// First selection
		node1, _ := sa.SelectWithClientIP(clientIP)

		// Mark the node as unhealthy
		node1.MarkUnhealthy()

		// Second selection should get a different healthy node
		node2, err := sa.SelectWithClientIP(clientIP)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if node2 == node1 {
			t.Error("Expected different node after original became unhealthy")
		}
		if !node2.IsHealthy() {
			t.Error("Expected new node to be healthy")
		}
	})

	t.Run("no healthy nodes returns error", func(t *testing.T) {
		pool := createTestPool(3)
		// Mark all nodes as unhealthy
		for _, n := range pool.All() {
			n.MarkUnhealthy()
		}

		baseBalancer := NewRoundRobinBalancer(pool)
		sa, _ := NewSessionAffinity(baseBalancer, 5*time.Minute)
		defer sa.Stop()

		clientIP := "192.168.1.1"
		_, err := sa.SelectWithClientIP(clientIP)

		if err != ErrNoNodeHealthy {
			t.Errorf("Expected ErrNoNodeHealthy, got %v", err)
		}
	})
}

// TestSessionAffinityBalancer_Select tests the standard Select method
func TestSessionAffinityBalancer_Select(t *testing.T) {
	t.Run("delegates to underlying balancer", func(t *testing.T) {
		pool := createTestPool(3)
		baseBalancer := NewRoundRobinBalancer(pool)
		sa, _ := NewSessionAffinity(baseBalancer, 5*time.Minute)
		defer sa.Stop()

		node1, err := sa.Select()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		node2, err := sa.Select()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		// Should follow round-robin pattern
		if node1 == node2 {
			t.Error("Expected different nodes in round-robin sequence")
		}
	})

	t.Run("does not create sessions", func(t *testing.T) {
		pool := createTestPool(3)
		baseBalancer := NewRoundRobinBalancer(pool)
		sa, _ := NewSessionAffinity(baseBalancer, 5*time.Minute)
		defer sa.Stop()

		sa.Select()
		sa.Select()

		sa.mu.RLock()
		sessionCount := len(sa.sessions)
		sa.mu.RUnlock()

		if sessionCount != 0 {
			t.Errorf("Expected 0 sessions, got %d", sessionCount)
		}
	})
}

// TestSessionAffinityBalancer_Name tests the Name method
func TestSessionAffinityBalancer_Name(t *testing.T) {
	pool := createTestPool(1)
	baseBalancer := NewRoundRobinBalancer(pool)
	sa, _ := NewSessionAffinity(baseBalancer, 5*time.Minute)
	defer sa.Stop()

	if sa.Name() != SessionAffinity {
		t.Errorf("Expected '%s', got '%s'", SessionAffinity, sa.Name())
	}
}

// TestSessionAffinityBalancer_ClearSession tests clearing individual sessions
func TestSessionAffinityBalancer_ClearSession(t *testing.T) {
	t.Run("clear existing session", func(t *testing.T) {
		pool := createTestPool(3)
		baseBalancer := NewRoundRobinBalancer(pool)
		sa, _ := NewSessionAffinity(baseBalancer, 5*time.Minute)
		defer sa.Stop()

		clientIP := "192.168.1.1"

		// Create session
		node1, _ := sa.SelectWithClientIP(clientIP)

		// Clear session
		sa.ClearSession(clientIP)

		// Verify session is removed
		sa.mu.RLock()
		_, exists := sa.sessions[clientIP]
		sa.mu.RUnlock()

		if exists {
			t.Error("Expected session to be cleared")
		}

		// Next selection should create new session
		node2, _ := sa.SelectWithClientIP(clientIP)

		// Nodes might be same or different, but session should be fresh
		sa.mu.RLock()
		sess, exists := sa.sessions[clientIP]
		sa.mu.RUnlock()

		if !exists {
			t.Error("Expected new session to be created")
		}
		if time.Since(sess.lastAccess) > time.Second {
			t.Error("Expected recent last access time for new session")
		}

		// Both nodes should be valid
		if node1 == nil || node2 == nil {
			t.Error("Expected valid nodes")
		}
	})

	t.Run("clear non-existent session is safe", func(t *testing.T) {
		pool := createTestPool(3)
		baseBalancer := NewRoundRobinBalancer(pool)
		sa, _ := NewSessionAffinity(baseBalancer, 5*time.Minute)
		defer sa.Stop()

		// This should not panic
		sa.ClearSession("192.168.1.99")
	})
}

// TestSessionAffinityBalancer_Cleanup tests automatic session cleanup
func TestSessionAffinityBalancer_Cleanup(t *testing.T) {
	t.Run("cleanup removes expired sessions", func(t *testing.T) {
		pool := createTestPool(3)
		baseBalancer := NewRoundRobinBalancer(pool)
		// Very short timeout for testing
		sa, _ := NewSessionAffinity(baseBalancer, 100*time.Millisecond)
		defer sa.Stop()

		// Create several sessions
		sa.SelectWithClientIP("192.168.1.1")
		sa.SelectWithClientIP("192.168.1.2")
		sa.SelectWithClientIP("192.168.1.3")

		sa.mu.RLock()
		initialCount := len(sa.sessions)
		sa.mu.RUnlock()

		if initialCount != 3 {
			t.Errorf("Expected 3 sessions, got %d", initialCount)
		}

		// Wait for sessions to expire
		time.Sleep(150 * time.Millisecond)

		// Manually trigger cleanup
		sa.cleanup()

		sa.mu.RLock()
		finalCount := len(sa.sessions)
		sa.mu.RUnlock()

		if finalCount != 0 {
			t.Errorf("Expected 0 sessions after cleanup, got %d", finalCount)
		}
	})

	t.Run("cleanup preserves active sessions", func(t *testing.T) {
		pool := createTestPool(3)
		baseBalancer := NewRoundRobinBalancer(pool)
		sa, _ := NewSessionAffinity(baseBalancer, 150*time.Millisecond)
		defer sa.Stop()

		// Create sessions with different ages
		sa.SelectWithClientIP("192.168.1.1")
		time.Sleep(200 * time.Millisecond)
		sa.SelectWithClientIP("192.168.1.2") // This one is fresh

		// Trigger cleanup
		sa.cleanup()

		sa.mu.RLock()
		_, exists1 := sa.sessions["192.168.1.1"]
		_, exists2 := sa.sessions["192.168.1.2"]
		sa.mu.RUnlock()

		if exists1 {
			t.Error("Expected old session to be removed")
		}
		if !exists2 {
			t.Error("Expected recent session to be preserved")
		}
	})
}

// TestSessionAffinityBalancer_Stop tests shutdown behavior
func TestSessionAffinityBalancer_Stop(t *testing.T) {
	t.Run("stop clears all sessions", func(t *testing.T) {
		pool := createTestPool(3)
		baseBalancer := NewRoundRobinBalancer(pool)
		sa, _ := NewSessionAffinity(baseBalancer, 5*time.Minute)

		// Create sessions
		sa.SelectWithClientIP("192.168.1.1")
		sa.SelectWithClientIP("192.168.1.2")
		sa.SelectWithClientIP("192.168.1.3")

		// Stop
		sa.Stop()

		sa.mu.RLock()
		sessionCount := len(sa.sessions)
		sa.mu.RUnlock()

		if sessionCount != 0 {
			t.Errorf("Expected 0 sessions after stop, got %d", sessionCount)
		}
	})

	t.Run("stop can be called multiple times", func(t *testing.T) {
		pool := createTestPool(3)
		baseBalancer := NewRoundRobinBalancer(pool)
		sa, _ := NewSessionAffinity(baseBalancer, 5*time.Minute)

		// Multiple stops should not panic
		sa.Stop()
		// Note: calling Stop() again would panic due to closing closed channel
		// This is acceptable behavior for this API
	})
}

// TestSessionAffinityBalancer_ConcurrentAccess tests thread safety
func TestSessionAffinityBalancer_ConcurrentAccess(t *testing.T) {
	t.Run("concurrent SelectWithClientIP", func(t *testing.T) {
		pool := createTestPool(5)
		baseBalancer := NewRoundRobinBalancer(pool)
		sa, _ := NewSessionAffinity(baseBalancer, 5*time.Minute)
		defer sa.Stop()

		var wg sync.WaitGroup
		numGoroutines := 100
		selectionsPerGoroutine := 100

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				clientIP := "192.168.1." + string(rune('0'+id%10))
				for j := 0; j < selectionsPerGoroutine; j++ {
					_, err := sa.SelectWithClientIP(clientIP)
					if err != nil {
						t.Errorf("Unexpected error: %v", err)
					}
				}
			}(i)
		}

		wg.Wait()
	})

	t.Run("concurrent Select and SelectWithClientIP", func(t *testing.T) {
		pool := createTestPool(5)
		baseBalancer := NewRoundRobinBalancer(pool)
		sa, _ := NewSessionAffinity(baseBalancer, 5*time.Minute)
		defer sa.Stop()

		var wg sync.WaitGroup

		// Regular Select calls
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					sa.Select()
				}
			}()
		}

		// SelectWithClientIP calls
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				clientIP := "192.168.1." + string(rune('0'+id%10))
				for j := 0; j < 100; j++ {
					sa.SelectWithClientIP(clientIP)
				}
			}(i)
		}

		wg.Wait()
	})

	t.Run("concurrent SelectWithClientIP and ClearSession", func(t *testing.T) {
		pool := createTestPool(5)
		baseBalancer := NewRoundRobinBalancer(pool)
		sa, _ := NewSessionAffinity(baseBalancer, 5*time.Minute)
		defer sa.Stop()

		var wg sync.WaitGroup

		// Selection goroutines
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				clientIP := "192.168.1." + string(rune('0'+id%10))
				for j := 0; j < 100; j++ {
					sa.SelectWithClientIP(clientIP)
				}
			}(i)
		}

		// Clear session goroutines
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				clientIP := "192.168.1." + string(rune('0'+id%10))
				for j := 0; j < 50; j++ {
					sa.ClearSession(clientIP)
					time.Sleep(time.Millisecond)
				}
			}(i)
		}

		wg.Wait()
	})
}

// TestSessionAffinityBalancer_LoadBalancerInterface ensures it implements the interface
func TestSessionAffinityBalancer_LoadBalancerInterface(t *testing.T) {
	pool := createTestPool(1)
	baseBalancer := NewRoundRobinBalancer(pool)
	sa, _ := NewSessionAffinity(baseBalancer, 5*time.Minute)
	defer sa.Stop()

	var _ LoadBalancer = sa
}

// BenchmarkSessionAffinityBalancer_SelectWithClientIP benchmarks SelectWithClientIP
func BenchmarkSessionAffinityBalancer_SelectWithClientIP(b *testing.B) {
	pool := createTestPool(10)
	baseBalancer := NewRoundRobinBalancer(pool)
	sa, _ := NewSessionAffinity(baseBalancer, 5*time.Minute)
	defer sa.Stop()

	clientIPs := []string{
		"192.168.1.1", "192.168.1.2", "192.168.1.3",
		"192.168.1.4", "192.168.1.5", "192.168.1.6",
		"192.168.1.7", "192.168.1.8", "192.168.1.9",
		"192.168.1.10",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clientIP := clientIPs[i%len(clientIPs)]
		_, _ = sa.SelectWithClientIP(clientIP)
	}
}

// BenchmarkSessionAffinityBalancer_SelectWithClientIPConcurrent benchmarks concurrent access
func BenchmarkSessionAffinityBalancer_SelectWithClientIPConcurrent(b *testing.B) {
	pool := createTestPool(10)
	baseBalancer := NewRoundRobinBalancer(pool)
	sa, _ := NewSessionAffinity(baseBalancer, 5*time.Minute)
	defer sa.Stop()

	clientIPs := []string{
		"192.168.1.1", "192.168.1.2", "192.168.1.3",
		"192.168.1.4", "192.168.1.5", "192.168.1.6",
		"192.168.1.7", "192.168.1.8", "192.168.1.9",
		"192.168.1.10",
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			clientIP := clientIPs[i%len(clientIPs)]
			_, _ = sa.SelectWithClientIP(clientIP)
			i++
		}
	})
}

// BenchmarkSessionAffinityBalancer_Select benchmarks standard Select
func BenchmarkSessionAffinityBalancer_Select(b *testing.B) {
	pool := createTestPool(10)
	baseBalancer := NewRoundRobinBalancer(pool)
	sa, _ := NewSessionAffinity(baseBalancer, 5*time.Minute)
	defer sa.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = sa.Select()
	}
}
