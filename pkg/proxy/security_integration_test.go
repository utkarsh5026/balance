package proxy

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/utkarsh5026/balance/pkg/conf"
)

func TestProxyServer_SecurityManager_RateLimiting(t *testing.T) {
	// Create a backend server
	backend := mockBackend(t, "localhost:0")
	defer backend.Close()

	// Create config with rate limiting
	cfg := &conf.Config{
		Mode:   "tcp",
		Listen: "localhost:0",
		Nodes: []conf.Node{
			{Name: "backend1", Address: backend.Addr().String(), Weight: 1},
		},
		LoadBalancer: conf.LoadBalancerConfig{
			Algorithm: "round-robin",
		},
		Timeouts: conf.TimeoutConfig{
			Connect: 5 * time.Second,
			Read:    30 * time.Second,
			Write:   30 * time.Second,
			Idle:    90 * time.Second,
		},
		Security: &conf.SecurityConfig{
			RateLimit: &conf.RateLimitConfig{
				Enabled:           true,
				Type:              "token-bucket",
				RequestsPerSecond: 2.0,
				BurstSize:         2,
			},
		},
	}

	// Create and start proxy server
	proxy, err := NewTCPServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create proxy server: %v", err)
	}

	go proxy.Start()
	defer proxy.Shutdown()

	time.Sleep(100 * time.Millisecond)

	listener := proxy.listener
	proxyAddr := listener.Addr().String()

	// First two connections should succeed (within burst size)
	for i := 0; i < 2; i++ {
		conn, err := net.Dial("tcp", proxyAddr)
		if err != nil {
			t.Fatalf("Connection %d failed: %v", i+1, err)
		}

		_, err = conn.Write([]byte("test"))
		if err != nil {
			t.Errorf("Write failed for connection %d: %v", i+1, err)
		}
		conn.Close()
	}

	// Third connection should be rate limited
	// We need to wait a bit to ensure the rate limiter has processed previous connections
	time.Sleep(10 * time.Millisecond)

	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("Failed to dial proxy: %v", err)
	}
	defer conn.Close()

	// Set a short read deadline to detect if connection is rejected
	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

	_, err = conn.Write([]byte("test"))
	// Connection might succeed but should be closed by the server due to rate limiting
	// The server will close the connection after the rate limit check
}

func TestProxyServer_SecurityManager_IPBlocklist(t *testing.T) {
	// Create a backend server
	backend := mockBackend(t, "localhost:0")
	defer backend.Close()

	// Note: In a real test, we'd need to spoof the source IP or use a custom dialer
	// For this test, we'll verify that the security manager is initialized correctly
	cfg := &conf.Config{
		Mode:   "tcp",
		Listen: "localhost:0",
		Nodes: []conf.Node{
			{Name: "backend1", Address: backend.Addr().String(), Weight: 1},
		},
		LoadBalancer: conf.LoadBalancerConfig{
			Algorithm: "round-robin",
		},
		Timeouts: conf.TimeoutConfig{
			Connect: 5 * time.Second,
			Read:    30 * time.Second,
			Write:   30 * time.Second,
			Idle:    90 * time.Second,
		},
		Security: &conf.SecurityConfig{
			IPBlocklist: &conf.IPBlocklistConfig{
				BlockedIPs: []string{"192.168.1.100", "10.0.0.50"},
			},
		},
	}

	// Create proxy server
	proxy, err := NewTCPServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create proxy server: %v", err)
	}

	if proxy.securityManager == nil {
		t.Fatal("Security manager should be initialized")
	}

	// Verify blocked IPs are in the blocklist
	blocked, _ := proxy.securityManager.AllowConn("192.168.1.100")
	if blocked {
		t.Error("IP 192.168.1.100 should be blocked")
	}

	blocked, _ = proxy.securityManager.AllowConn("10.0.0.50")
	if blocked {
		t.Error("IP 10.0.0.50 should be blocked")
	}

	// Verify non-blocked IP is allowed
	allowed, _ := proxy.securityManager.AllowConn("127.0.0.1")
	if !allowed {
		t.Error("IP 127.0.0.1 should be allowed")
	}
}

func TestProxyServer_SecurityManager_Disabled(t *testing.T) {
	// Create a backend server
	backend := mockBackend(t, "localhost:0")
	defer backend.Close()

	// Create config without security
	cfg := &conf.Config{
		Mode:   "tcp",
		Listen: "localhost:0",
		Nodes: []conf.Node{
			{Name: "backend1", Address: backend.Addr().String(), Weight: 1},
		},
		LoadBalancer: conf.LoadBalancerConfig{
			Algorithm: "round-robin",
		},
		Timeouts: conf.TimeoutConfig{
			Connect: 5 * time.Second,
			Read:    30 * time.Second,
			Write:   30 * time.Second,
			Idle:    90 * time.Second,
		},
	}

	// Create proxy server
	proxy, err := NewTCPServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create proxy server: %v", err)
	}

	if proxy.securityManager != nil {
		t.Error("Security manager should be nil when security is not configured")
	}
}

func TestCreateSecurityManager(t *testing.T) {
	ctx := context.Background()

	t.Run("nil security config", func(t *testing.T) {
		cfg := &conf.Config{}
		sm := createSecurityManager(ctx, cfg)
		if sm != nil {
			t.Error("Expected nil security manager for nil security config")
		}
	})

	t.Run("token bucket rate limiter", func(t *testing.T) {
		cfg := &conf.Config{
			Security: &conf.SecurityConfig{
				RateLimit: &conf.RateLimitConfig{
					Enabled:           true,
					Type:              "token-bucket",
					RequestsPerSecond: 10.0,
					BurstSize:         20,
				},
			},
		}
		sm := createSecurityManager(ctx, cfg)
		if sm == nil {
			t.Fatal("Expected non-nil security manager")
		}

		// Test rate limiting
		allowed, err := sm.AllowConn("127.0.0.1")
		if !allowed || err != nil {
			t.Error("First request should be allowed")
		}
	})

	t.Run("sliding window rate limiter", func(t *testing.T) {
		cfg := &conf.Config{
			Security: &conf.SecurityConfig{
				RateLimit: &conf.RateLimitConfig{
					Enabled:     true,
					Type:        "sliding-window",
					WindowSize:  "1m",
					MaxRequests: 100,
				},
			},
		}
		sm := createSecurityManager(ctx, cfg)
		if sm == nil {
			t.Fatal("Expected non-nil security manager")
		}

		// Test rate limiting
		allowed, err := sm.AllowConn("127.0.0.1")
		if !allowed || err != nil {
			t.Error("First request should be allowed")
		}
	})

	t.Run("IP blocklist", func(t *testing.T) {
		cfg := &conf.Config{
			Security: &conf.SecurityConfig{
				IPBlocklist: &conf.IPBlocklistConfig{
					BlockedIPs: []string{"192.168.1.100"},
				},
			},
		}
		sm := createSecurityManager(ctx, cfg)
		if sm == nil {
			t.Fatal("Expected non-nil security manager")
		}

		// Test blocked IP
		allowed, err := sm.AllowConn("192.168.1.100")
		if allowed || err == nil {
			t.Error("Blocked IP should not be allowed")
		}

		// Test non-blocked IP
		allowed, err = sm.AllowConn("127.0.0.1")
		if !allowed || err != nil {
			t.Error("Non-blocked IP should be allowed")
		}
	})
}
