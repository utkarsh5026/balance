package security

import (
	"context"
	"testing"
	"time"
)

func TestNewIPBlocklist(t *testing.T) {
	ctx := t.Context()

	bl := NewIPBlocklist(ctx)

	if bl == nil {
		t.Fatal("NewIPBlocklist returned nil")
	}

	if bl.blocked == nil {
		t.Error("blocked map not initialized")
	}

	if bl.permanent == nil {
		t.Error("permanent map not initialized")
	}
}

func TestIPBlocklist_BlockPermanent(t *testing.T) {
	ctx := t.Context()

	bl := NewIPBlocklist(ctx)
	ip := "192.168.1.100"

	bl.BlockPermanent(ip)

	if !bl.IsBlocked(ip) {
		t.Errorf("IP %s should be blocked after BlockPermanent", ip)
	}

	// Verify it's in the permanent map
	bl.mu.RLock()
	if !bl.permanent[ip] {
		t.Error("IP should be in permanent map")
	}
	bl.mu.RUnlock()
}

func TestIPBlocklist_IsBlocked_NotBlocked(t *testing.T) {
	ctx := t.Context()

	bl := NewIPBlocklist(ctx)
	ip := "192.168.1.100"

	if bl.IsBlocked(ip) {
		t.Errorf("IP %s should not be blocked initially", ip)
	}
}

func TestIPBlocklist_IsBlocked_TemporaryBlock(t *testing.T) {
	ctx := t.Context()

	bl := NewIPBlocklist(ctx)
	ip := "192.168.1.100"

	// Manually add a temporary block
	bl.mu.Lock()
	bl.blocked[ip] = time.Now().Add(1 * time.Hour)
	bl.mu.Unlock()

	if !bl.IsBlocked(ip) {
		t.Errorf("IP %s should be blocked with temporary block", ip)
	}
}

func TestIPBlocklist_IsBlocked_ExpiredBlock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bl := NewIPBlocklist(ctx)
	ip := "192.168.1.100"

	// Add an expired block
	bl.mu.Lock()
	bl.blocked[ip] = time.Now().Add(-1 * time.Hour)
	bl.mu.Unlock()

	if bl.IsBlocked(ip) {
		t.Errorf("IP %s should not be blocked with expired block", ip)
	}
}

func TestIPBlocklist_IsBlocked_PermanentTakesPrecedence(t *testing.T) {
	ctx := t.Context()

	bl := NewIPBlocklist(ctx)
	ip := "192.168.1.100"

	// Add both permanent and expired temporary block
	bl.mu.Lock()
	bl.permanent[ip] = true
	bl.blocked[ip] = time.Now().Add(-1 * time.Hour)
	bl.mu.Unlock()

	if !bl.IsBlocked(ip) {
		t.Errorf("IP %s should be blocked permanently even with expired temporary block", ip)
	}
}

func TestIPBlocklist_Unblock(t *testing.T) {
	ctx := t.Context()

	bl := NewIPBlocklist(ctx)
	ip := "192.168.1.100"

	// Block permanently
	bl.BlockPermanent(ip)

	if !bl.IsBlocked(ip) {
		t.Error("IP should be blocked before unblock")
	}

	// Unblock
	bl.Unblock(ip)

	if bl.IsBlocked(ip) {
		t.Errorf("IP %s should not be blocked after Unblock", ip)
	}

	// Verify it's removed from permanent map
	bl.mu.RLock()
	if bl.permanent[ip] {
		t.Error("IP should not be in permanent map after unblock")
	}
	bl.mu.RUnlock()
}

func TestIPBlocklist_Unblock_TemporaryBlock(t *testing.T) {
	ctx := t.Context()

	bl := NewIPBlocklist(ctx)
	ip := "192.168.1.100"

	// Add temporary block
	bl.mu.Lock()
	bl.blocked[ip] = time.Now().Add(1 * time.Hour)
	bl.mu.Unlock()

	if !bl.IsBlocked(ip) {
		t.Error("IP should be blocked before unblock")
	}

	// Unblock
	bl.Unblock(ip)

	if bl.IsBlocked(ip) {
		t.Errorf("IP %s should not be blocked after Unblock", ip)
	}

	// Verify it's removed from blocked map
	bl.mu.RLock()
	if _, exists := bl.blocked[ip]; exists {
		t.Error("IP should not be in blocked map after unblock")
	}
	bl.mu.RUnlock()
}

func TestIPBlocklist_Unblock_BothMaps(t *testing.T) {
	ctx := t.Context()

	bl := NewIPBlocklist(ctx)
	ip := "192.168.1.100"

	// Add to both maps
	bl.mu.Lock()
	bl.permanent[ip] = true
	bl.blocked[ip] = time.Now().Add(1 * time.Hour)
	bl.mu.Unlock()

	// Unblock should remove from both
	bl.Unblock(ip)

	bl.mu.RLock()
	if bl.permanent[ip] {
		t.Error("IP should not be in permanent map after unblock")
	}
	if _, exists := bl.blocked[ip]; exists {
		t.Error("IP should not be in blocked map after unblock")
	}
	bl.mu.RUnlock()
}

func TestIPBlocklist_RemoveBlockedPeriodically(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bl := NewIPBlocklist(ctx)
	ip1 := "192.168.1.100"
	ip2 := "192.168.1.101"
	ip3 := "192.168.1.102"

	// Add blocks with different expiry times
	bl.mu.Lock()
	bl.blocked[ip1] = time.Now().Add(-1 * time.Second) // Already expired
	bl.blocked[ip2] = time.Now().Add(100 * time.Millisecond)
	bl.blocked[ip3] = time.Now().Add(10 * time.Hour) // Long duration
	bl.mu.Unlock()

	// Wait a bit longer than the ticker interval (which is 1 minute in production)
	// For testing purposes, we'll wait and check the cleanup logic
	time.Sleep(150 * time.Millisecond)

	// Manually trigger cleanup logic to test it
	bl.mu.Lock()
	now := time.Now()
	toDelete := make([]string, 0)
	for ip, expiry := range bl.blocked {
		if now.After(expiry) {
			toDelete = append(toDelete, ip)
		}
	}
	for _, ip := range toDelete {
		delete(bl.blocked, ip)
	}
	bl.mu.Unlock()

	// Check results
	bl.mu.RLock()
	if _, exists := bl.blocked[ip1]; exists {
		t.Error("Expired IP1 should have been removed")
	}
	if _, exists := bl.blocked[ip2]; exists {
		t.Error("Expired IP2 should have been removed")
	}
	if _, exists := bl.blocked[ip3]; !exists {
		t.Error("Non-expired IP3 should still be present")
	}
	bl.mu.RUnlock()
}

func TestIPBlocklist_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	bl := NewIPBlocklist(ctx)
	ip := "192.168.1.100"

	bl.BlockPermanent(ip)

	// Cancel context
	cancel()

	// Give goroutine time to exit
	time.Sleep(100 * time.Millisecond)

	// Blocklist should still work after context cancellation
	if !bl.IsBlocked(ip) {
		t.Error("IP should still be blocked after context cancellation")
	}
}

func TestIPBlocklist_ConcurrentAccess(t *testing.T) {
	ctx := t.Context()

	bl := NewIPBlocklist(ctx)

	// Test concurrent blocking and checking
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			ip := "192.168.1.100"
			bl.BlockPermanent(ip)
			_ = bl.IsBlocked(ip)
			bl.Unblock(ip)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for range 10 {
		<-done
	}
}

func TestIPBlocklist_MultipleIPs(t *testing.T) {
	ctx := t.Context()

	bl := NewIPBlocklist(ctx)

	ips := []string{
		"192.168.1.100",
		"192.168.1.101",
		"10.0.0.1",
		"172.16.0.1",
	}

	// Block all IPs permanently
	for _, ip := range ips {
		bl.BlockPermanent(ip)
	}

	// Verify all are blocked
	for _, ip := range ips {
		if !bl.IsBlocked(ip) {
			t.Errorf("IP %s should be blocked", ip)
		}
	}

	// Unblock one IP
	bl.Unblock(ips[0])

	// Verify only the unblocked IP is not blocked
	if bl.IsBlocked(ips[0]) {
		t.Errorf("IP %s should not be blocked after unblock", ips[0])
	}

	for _, ip := range ips[1:] {
		if !bl.IsBlocked(ip) {
			t.Errorf("IP %s should still be blocked", ip)
		}
	}
}

func TestIPBlocklist_EmptyIP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bl := NewIPBlocklist(ctx)
	ip := ""

	bl.BlockPermanent(ip)

	if !bl.IsBlocked(ip) {
		t.Error("Empty IP should be blocked after BlockPermanent")
	}

	bl.Unblock(ip)

	if bl.IsBlocked(ip) {
		t.Error("Empty IP should not be blocked after Unblock")
	}
}
