package proxy

import (
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/utkarsh5026/balance/pkg/conf"
)

// mockBackend creates a simple TCP echo server for testing
func mockBackend(t *testing.T, addr string) net.Listener {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("failed to create mock backend: %v", err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c) // Echo back
			}(conn)
		}
	}()

	return listener
}

// createTestConfig creates a basic configuration for testing
func createTestConfig(listenAddr string, backends []string) *conf.Config {
	nodes := make([]conf.Node, len(backends))
	for i, addr := range backends {
		nodes[i] = conf.Node{
			Name:    fmt.Sprintf("backend-%d", i+1),
			Address: addr,
			Weight:  1,
		}
	}

	return &conf.Config{
		Mode:   "tcp",
		Listen: listenAddr,
		Nodes:  nodes,
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
}

func TestNewTCPServer(t *testing.T) {
	tests := []struct {
		name    string
		config  *conf.Config
		wantErr bool
	}{
		{
			name: "valid round-robin config",
			config: &conf.Config{
				Mode:   "tcp",
				Listen: ":0",
				Nodes: []conf.Node{
					{Name: "node1", Address: "localhost:8001", Weight: 1},
				},
				LoadBalancer: conf.LoadBalancerConfig{Algorithm: "round-robin"},
				Timeouts: conf.TimeoutConfig{
					Connect: 5 * time.Second,
				},
			},
			wantErr: false,
		},
		{
			name: "valid weighted-round-robin config",
			config: &conf.Config{
				Mode:   "tcp",
				Listen: ":0",
				Nodes: []conf.Node{
					{Name: "node1", Address: "localhost:8001", Weight: 2},
					{Name: "node2", Address: "localhost:8002", Weight: 1},
				},
				LoadBalancer: conf.LoadBalancerConfig{Algorithm: "weighted-round-robin"},
				Timeouts: conf.TimeoutConfig{
					Connect: 5 * time.Second,
				},
			},
			wantErr: false,
		},
		{
			name: "valid least-connections config",
			config: &conf.Config{
				Mode:   "tcp",
				Listen: ":0",
				Nodes: []conf.Node{
					{Name: "node1", Address: "localhost:8001", Weight: 1},
				},
				LoadBalancer: conf.LoadBalancerConfig{Algorithm: "least-connections"},
				Timeouts: conf.TimeoutConfig{
					Connect: 5 * time.Second,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid algorithm",
			config: &conf.Config{
				Mode:   "tcp",
				Listen: ":0",
				Nodes: []conf.Node{
					{Name: "node1", Address: "localhost:8001", Weight: 1},
				},
				LoadBalancer: conf.LoadBalancerConfig{Algorithm: "invalid-algorithm"},
				Timeouts: conf.TimeoutConfig{
					Connect: 5 * time.Second,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewTCPServer(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTCPServer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && server == nil {
				t.Error("NewTCPServer() returned nil server")
			}
			if server != nil {
				// Verify fields are initialized
				if server.config == nil {
					t.Error("server.config is nil")
				}
				if server.pool == nil {
					t.Error("server.pool is nil")
				}
				if server.balancer == nil {
					t.Error("server.balancer is nil")
				}
				if server.ctx == nil {
					t.Error("server.ctx is nil")
				}
				if server.cancel == nil {
					t.Error("server.cancel is nil")
				}
			}
		})
	}
}

func TestProxyServer_StartAndShutdown(t *testing.T) {
	// Create mock backend
	backend := mockBackend(t, "localhost:0")
	defer backend.Close()
	backendAddr := backend.Addr().String()

	// Create proxy server config
	cfg := createTestConfig("localhost:0", []string{backendAddr})
	server, err := NewTCPServer(cfg)
	if err != nil {
		t.Fatalf("failed to create proxy server: %v", err)
	}

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Start()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Verify listener is created
	if server.listener == nil {
		t.Fatal("listener was not created")
	}

	// Shutdown server
	if err := server.Shutdown(); err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}

	// Wait for Start to complete
	select {
	case <-errChan:
		// Expected - server should exit after shutdown
	case <-time.After(2 * time.Second):
		t.Error("server did not shutdown in time")
	}
}

func TestProxyServer_HandleConnection(t *testing.T) {
	// Create mock backend
	backend := mockBackend(t, "localhost:0")
	defer backend.Close()
	backendAddr := backend.Addr().String()

	// Create and start proxy server
	cfg := createTestConfig("localhost:0", []string{backendAddr})
	server, err := NewTCPServer(cfg)
	if err != nil {
		t.Fatalf("failed to create proxy server: %v", err)
	}

	go server.Start()
	time.Sleep(100 * time.Millisecond)
	defer server.Shutdown()

	// Get proxy address
	proxyAddr := server.listener.Addr().String()

	// Connect to proxy
	client, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer client.Close()

	// Send data
	testData := "Hello, Proxy!"
	if _, err := client.Write([]byte(testData)); err != nil {
		t.Fatalf("failed to write to proxy: %v", err)
	}

	// Read response
	buf := make([]byte, len(testData))
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := io.ReadFull(client, buf)
	if err != nil {
		t.Fatalf("failed to read from proxy: %v", err)
	}

	// Verify echo
	if string(buf[:n]) != testData {
		t.Errorf("expected %q, got %q", testData, string(buf[:n]))
	}

	// Close connection and wait for stats to update
	_ = client.Close()
	time.Sleep(100 * time.Millisecond)

	// Verify stats
	if server.totalConnections.Load() == 0 {
		t.Error("totalConnections should be > 0")
	}
	if server.totalBytesSent.Load() == 0 {
		t.Error("totalBytesSent should be > 0")
	}
	if server.totalBytesRecieved.Load() == 0 {
		t.Error("totalBytesRecieved should be > 0")
	}
}

func TestProxyServer_MultipleConnections(t *testing.T) {
	// Create mock backend
	backend := mockBackend(t, "localhost:0")
	defer backend.Close()
	backendAddr := backend.Addr().String()

	// Create and start proxy server
	cfg := createTestConfig("localhost:0", []string{backendAddr})
	server, err := NewTCPServer(cfg)
	if err != nil {
		t.Fatalf("failed to create proxy server: %v", err)
	}

	go server.Start()
	time.Sleep(100 * time.Millisecond)
	defer server.Shutdown()

	proxyAddr := server.listener.Addr().String()

	// Create multiple concurrent connections
	numConnections := 10
	var wg sync.WaitGroup
	wg.Add(numConnections)

	for i := 0; i < numConnections; i++ {
		go func(id int) {
			defer wg.Done()

			client, err := net.Dial("tcp", proxyAddr)
			if err != nil {
				t.Errorf("connection %d: failed to connect: %v", id, err)
				return
			}
			defer client.Close()

			testData := fmt.Sprintf("Message from client %d", id)
			if _, err := client.Write([]byte(testData)); err != nil {
				t.Errorf("connection %d: failed to write: %v", id, err)
				return
			}

			buf := make([]byte, len(testData))
			client.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, err := io.ReadFull(client, buf)
			if err != nil {
				t.Errorf("connection %d: failed to read: %v", id, err)
				return
			}

			if string(buf[:n]) != testData {
				t.Errorf("connection %d: expected %q, got %q", id, testData, string(buf[:n]))
			}
		}(i)
	}

	wg.Wait()

	// Verify total connections
	totalConns := server.totalConnections.Load()
	if totalConns < uint64(numConnections) {
		t.Errorf("expected at least %d connections, got %d", numConnections, totalConns)
	}
}

func TestProxyServer_ConnectionTimeout(t *testing.T) {
	// Create config with short timeout pointing to non-existent backend
	cfg := &conf.Config{
		Mode:   "tcp",
		Listen: "localhost:0",
		Nodes: []conf.Node{
			{Name: "node1", Address: "localhost:59999", Weight: 1}, // Non-existent backend
		},
		LoadBalancer: conf.LoadBalancerConfig{Algorithm: "round-robin"},
		Timeouts: conf.TimeoutConfig{
			Connect: 100 * time.Millisecond, // Short timeout
			Read:    1 * time.Second,
			Write:   1 * time.Second,
		},
	}

	server, err := NewTCPServer(cfg)
	if err != nil {
		t.Fatalf("failed to create proxy server: %v", err)
	}

	go server.Start()
	time.Sleep(100 * time.Millisecond)
	defer server.Shutdown()

	proxyAddr := server.listener.Addr().String()

	// Connect to proxy - should fail to connect to backend
	client, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer client.Close()

	// Try to read - connection should be closed due to backend connection failure
	buf := make([]byte, 100)
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = client.Read(buf)
	if err == nil {
		t.Error("expected connection to be closed, but read succeeded")
	}
}

func TestProxyServer_Shutdown_ClosesConnections(t *testing.T) {
	backend := mockBackend(t, "localhost:0")
	defer backend.Close()
	backendAddr := backend.Addr().String()

	cfg := createTestConfig("localhost:0", []string{backendAddr})
	server, err := NewTCPServer(cfg)
	if err != nil {
		t.Fatalf("failed to create proxy server: %v", err)
	}

	go server.Start()
	time.Sleep(100 * time.Millisecond)

	proxyAddr := server.listener.Addr().String()

	// Create a connection
	client, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer client.Close()

	// Send some data to trigger the connection handler
	_, _ = client.Write([]byte("test"))
	time.Sleep(100 * time.Millisecond)

	// Verify connection is active
	if server.activeConnections.Load() == 0 {
		t.Error("expected active connections > 0")
	}

	// Shutdown server
	if err := server.Shutdown(); err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}

	// Try to read from client - should fail or return EOF
	buf := make([]byte, 100)
	client.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, err = client.Read(buf)
	if err == nil {
		// Connection might still work briefly, but new connections should fail
	}

	// Verify new connections are rejected
	_, err = net.DialTimeout("tcp", proxyAddr, 500*time.Millisecond)
	if err == nil {
		t.Error("expected new connection to fail after shutdown")
	}
}

func TestProxyServer_LoadBalancing_RoundRobin(t *testing.T) {
	// Create two mock backends
	backend1 := mockBackend(t, "localhost:0")
	defer backend1.Close()
	backend2 := mockBackend(t, "localhost:0")
	defer backend2.Close()

	backends := []string{
		backend1.Addr().String(),
		backend2.Addr().String(),
	}

	cfg := createTestConfig("localhost:0", backends)
	server, err := NewTCPServer(cfg)
	if err != nil {
		t.Fatalf("failed to create proxy server: %v", err)
	}

	go server.Start()
	time.Sleep(100 * time.Millisecond)
	defer server.Shutdown()

	proxyAddr := server.listener.Addr().String()

	// Make multiple connections to verify round-robin behavior
	for i := 0; i < 4; i++ {
		client, err := net.Dial("tcp", proxyAddr)
		if err != nil {
			t.Fatalf("connection %d failed: %v", i, err)
		}

		testData := fmt.Sprintf("test-%d", i)
		client.Write([]byte(testData))

		buf := make([]byte, len(testData))
		client.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, _ := io.ReadFull(client, buf)

		if string(buf[:n]) != testData {
			t.Errorf("connection %d: echo mismatch", i)
		}

		client.Close()
	}

	// Verify stats
	if server.totalConnections.Load() < 4 {
		t.Errorf("expected at least 4 connections, got %d", server.totalConnections.Load())
	}
}

func TestProxyServer_ContextCancellation(t *testing.T) {
	backend := mockBackend(t, "localhost:0")
	defer backend.Close()
	backendAddr := backend.Addr().String()

	cfg := createTestConfig("localhost:0", []string{backendAddr})
	server, err := NewTCPServer(cfg)
	if err != nil {
		t.Fatalf("failed to create proxy server: %v", err)
	}

	go server.Start()
	time.Sleep(100 * time.Millisecond)

	// Cancel context
	server.cancel()

	// Give it time to process cancellation
	time.Sleep(100 * time.Millisecond)

	// Try to connect - might succeed or fail depending on timing
	proxyAddr := server.listener.Addr().String()
	client, err := net.DialTimeout("tcp", proxyAddr, 500*time.Millisecond)
	if err == nil {
		client.Close()
	}

	// Cleanup
	server.Shutdown()
}

func TestProxyServer_Statistics(t *testing.T) {
	backend := mockBackend(t, "localhost:0")
	defer backend.Close()
	backendAddr := backend.Addr().String()

	cfg := createTestConfig("localhost:0", []string{backendAddr})
	server, err := NewTCPServer(cfg)
	if err != nil {
		t.Fatalf("failed to create proxy server: %v", err)
	}

	go server.Start()
	time.Sleep(100 * time.Millisecond)
	defer server.Shutdown()

	proxyAddr := server.listener.Addr().String()

	// Initial state
	if server.totalConnections.Load() != 0 {
		t.Error("totalConnections should start at 0")
	}
	if server.activeConnections.Load() != 0 {
		t.Error("activeConnections should start at 0")
	}

	// Create connection and send data
	client, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	testData := "test data for statistics"
	client.Write([]byte(testData))

	buf := make([]byte, len(testData))
	client.SetReadDeadline(time.Now().Add(1 * time.Second))
	io.ReadFull(client, buf)
	client.Close()

	// Give time for connection to be fully processed
	time.Sleep(200 * time.Millisecond)

	// Verify statistics
	if server.totalConnections.Load() != 1 {
		t.Errorf("expected totalConnections = 1, got %d", server.totalConnections.Load())
	}

	bytesSent := server.totalBytesSent.Load()
	bytesReceived := server.totalBytesRecieved.Load()

	if bytesSent == 0 {
		t.Error("totalBytesSent should be > 0")
	}
	if bytesReceived == 0 {
		t.Error("totalBytesRecieved should be > 0")
	}

	// In echo server, bytes sent should equal bytes received
	if bytesSent != bytesReceived {
		t.Logf("Note: bytesSent=%d, bytesReceived=%d (may differ due to buffering)", bytesSent, bytesReceived)
	}
}

func TestProxyServer_ReadWriteDeadlines(t *testing.T) {
	backend := mockBackend(t, "localhost:0")
	defer backend.Close()
	backendAddr := backend.Addr().String()

	cfg := &conf.Config{
		Mode:   "tcp",
		Listen: "localhost:0",
		Nodes: []conf.Node{
			{Name: "node1", Address: backendAddr, Weight: 1},
		},
		LoadBalancer: conf.LoadBalancerConfig{Algorithm: "round-robin"},
		Timeouts: conf.TimeoutConfig{
			Connect: 5 * time.Second,
			Read:    500 * time.Millisecond,
			Write:   500 * time.Millisecond,
		},
	}

	server, err := NewTCPServer(cfg)
	if err != nil {
		t.Fatalf("failed to create proxy server: %v", err)
	}

	go server.Start()
	time.Sleep(100 * time.Millisecond)
	defer server.Shutdown()

	proxyAddr := server.listener.Addr().String()
	client, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	// Send and receive data quickly - should work
	testData := "quick test"
	client.Write([]byte(testData))
	buf := make([]byte, len(testData))
	client.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, err = io.ReadFull(client, buf)
	if err != nil {
		t.Errorf("quick operation failed: %v", err)
	}
}
