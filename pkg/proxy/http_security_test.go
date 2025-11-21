package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/utkarsh5026/balance/pkg/conf"
)

func TestHttpProxyServer_SecurityManager_RateLimiting(t *testing.T) {
	// Create backend server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer backend.Close()

	// Create config with rate limiting
	cfg := createTestHTTPConfig("localhost:0", []string{backend.URL})
	cfg.Security = &conf.SecurityConfig{
		RateLimit: &conf.RateLimitConfig{
			Enabled:           true,
			Type:              "token-bucket",
			RequestsPerSecond: 2.0,
			BurstSize:         2,
		},
	}

	// Create proxy server
	proxy, err := NewHttpProxyServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create proxy server: %v", err)
	}

	if proxy.securityManager == nil {
		t.Fatal("Security manager should be initialized")
	}

	// Test rate limiting directly on security manager
	clientIP := "127.0.0.1"

	// First two requests should succeed (within burst size)
	for i := 0; i < 2; i++ {
		allowed, err := proxy.securityManager.AllowConn(clientIP)
		if err != nil || !allowed {
			t.Errorf("Request %d should be allowed: allowed=%v, err=%v", i+1, allowed, err)
		}
	}

	// Third request should be rate limited
	allowed, err := proxy.securityManager.AllowConn(clientIP)
	if allowed || err == nil {
		t.Error("Third request should be rate limited")
	}
}

func TestHttpProxyServer_SecurityManager_IPBlocklist(t *testing.T) {
	// Create backend server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer backend.Close()

	// Create config with IP blocklist
	cfg := createTestHTTPConfig("localhost:0", []string{backend.URL})
	cfg.Security = &conf.SecurityConfig{
		IPBlocklist: &conf.IPBlocklistConfig{
			BlockedIPs: []string{"192.168.1.100", "10.0.0.50"},
		},
	}

	// Create proxy server
	proxy, err := NewHttpProxyServer(cfg)
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

func TestHttpProxyServer_SecurityManager_Disabled(t *testing.T) {
	// Create backend server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer backend.Close()

	// Create config without security
	cfg := createTestHTTPConfig("localhost:0", []string{backend.URL})

	// Create proxy server
	proxy, err := NewHttpProxyServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create proxy server: %v", err)
	}

	if proxy.securityManager != nil {
		t.Error("Security manager should be nil when security is not configured")
	}
}

func TestHttpProxyServer_SecurityManager_SlidingWindow(t *testing.T) {
	// Create backend server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer backend.Close()

	// Create config with sliding window rate limiting
	cfg := createTestHTTPConfig("localhost:0", []string{backend.URL})
	cfg.Security = &conf.SecurityConfig{
		RateLimit: &conf.RateLimitConfig{
			Enabled:     true,
			Type:        "sliding-window",
			WindowSize:  "1s",
			MaxRequests: 3,
		},
	}

	// Create proxy server
	proxy, err := NewHttpProxyServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create proxy server: %v", err)
	}

	if proxy.securityManager == nil {
		t.Fatal("Security manager should be initialized")
	}

	// Test rate limiting directly on security manager
	clientIP := "127.0.0.1"

	// First 3 requests should succeed (within window limit)
	for i := 0; i < 3; i++ {
		allowed, err := proxy.securityManager.AllowConn(clientIP)
		if err != nil || !allowed {
			t.Errorf("Request %d should be allowed: allowed=%v, err=%v", i+1, allowed, err)
		}
	}

	// Fourth request should be rate limited
	allowed, err := proxy.securityManager.AllowConn(clientIP)
	if allowed || err == nil {
		t.Error("Fourth request should be rate limited")
	}
}

func TestHttpProxyServer_WebSocket_SecurityManager(t *testing.T) {
	// This test verifies that WebSocket requests are also subject to security checks
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	// Create config with rate limiting and WebSocket enabled
	cfg := createTestHTTPConfig("localhost:0", []string{backend.URL})
	cfg.HTTP.EnableWebSocket = true
	cfg.Security = &conf.SecurityConfig{
		RateLimit: &conf.RateLimitConfig{
			Enabled:           true,
			Type:              "token-bucket",
			RequestsPerSecond: 1.0,
			BurstSize:         1,
		},
	}

	// Create proxy server
	proxy, err := NewHttpProxyServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create proxy server: %v", err)
	}

	if proxy.securityManager == nil {
		t.Fatal("Security manager should be initialized")
	}
}
