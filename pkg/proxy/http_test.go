package proxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/utkarsh5026/balance/pkg/conf"
)

// mockHTTPBackend creates a simple HTTP test server
func mockHTTPBackend(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	if handler == nil {
		handler = func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Hello from backend"))
		}
	}
	return httptest.NewServer(handler)
}

// createTestHTTPConfig creates a basic HTTP configuration for testing
func createTestHTTPConfig(listenAddr string, backends []string) *conf.Config {
	nodes := make([]conf.Node, len(backends))
	for i, addr := range backends {
		nodes[i] = conf.Node{
			Name:    fmt.Sprintf("backend-%d", i+1),
			Address: addr,
			Weight:  1,
		}
	}

	return &conf.Config{
		Mode:   "http",
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
		HTTP: &conf.HTTPConfig{
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			EnableHTTP2:         false,
			EnableWebSocket:     false,
		},
	}
}

// Helper to start HTTP proxy server with actual listener
func startTestHTTPServer(t *testing.T, cfg *conf.Config) (*HttpProxyServer, string) {
	server, err := NewHttpProxyServer(cfg)
	if err != nil {
		t.Fatalf("failed to create HTTP proxy server: %v", err)
	}

	// Create a listener to get actual port
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	// Start server in background
	go func() {
		_ = server.server.Serve(listener)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Return server and actual address
	return server, listener.Addr().String()
}

func TestNewHttpProxyServer(t *testing.T) {
	tests := []struct {
		name    string
		config  *conf.Config
		wantErr bool
	}{
		{
			name: "valid HTTP config with round-robin",
			config: &conf.Config{
				Mode:   "http",
				Listen: "localhost:0",
				Nodes: []conf.Node{
					{Name: "node1", Address: "localhost:8001", Weight: 1},
				},
				LoadBalancer: conf.LoadBalancerConfig{Algorithm: "round-robin"},
				Timeouts: conf.TimeoutConfig{
					Connect: 5 * time.Second,
					Read:    30 * time.Second,
					Write:   30 * time.Second,
					Idle:    90 * time.Second,
				},
				HTTP: &conf.HTTPConfig{
					MaxIdleConnsPerHost: 10,
					IdleConnTimeout:     90 * time.Second,
				},
			},
			wantErr: false,
		},
		{
			name: "valid HTTP config with weighted round-robin",
			config: &conf.Config{
				Mode:   "http",
				Listen: "localhost:0",
				Nodes: []conf.Node{
					{Name: "node1", Address: "localhost:8001", Weight: 2},
					{Name: "node2", Address: "localhost:8002", Weight: 1},
				},
				LoadBalancer: conf.LoadBalancerConfig{Algorithm: "weighted-round-robin"},
				Timeouts: conf.TimeoutConfig{
					Connect: 5 * time.Second,
					Read:    30 * time.Second,
					Write:   30 * time.Second,
					Idle:    90 * time.Second,
				},
				HTTP: &conf.HTTPConfig{
					MaxIdleConnsPerHost: 10,
					IdleConnTimeout:     90 * time.Second,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid algorithm",
			config: &conf.Config{
				Mode:   "http",
				Listen: "localhost:0",
				Nodes: []conf.Node{
					{Name: "node1", Address: "localhost:8001", Weight: 1},
				},
				LoadBalancer: conf.LoadBalancerConfig{Algorithm: "invalid-algorithm"},
				Timeouts: conf.TimeoutConfig{
					Connect: 5 * time.Second,
					Read:    30 * time.Second,
					Write:   30 * time.Second,
					Idle:    90 * time.Second,
				},
				HTTP: &conf.HTTPConfig{
					MaxIdleConnsPerHost: 10,
					IdleConnTimeout:     90 * time.Second,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewHttpProxyServer(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewHttpProxyServer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && server == nil {
				t.Error("NewHttpProxyServer() returned nil server")
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
				if server.transport == nil {
					t.Error("server.transport is nil")
				}
				if server.ctx == nil {
					t.Error("server.ctx is nil")
				}
				if server.cancelFunc == nil {
					t.Error("server.cancelFunc is nil")
				}
			}
		})
	}
}

func TestHttpProxyServer_HandleRequest(t *testing.T) {
	// Create mock backend
	backend := mockHTTPBackend(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Backend response"))
	})
	defer backend.Close()

	// Parse backend address to get host:port
	backendAddr := strings.TrimPrefix(backend.URL, "http://")

	// Create and start proxy server
	cfg := createTestHTTPConfig("127.0.0.1:0", []string{backendAddr})
	server, proxyAddr := startTestHTTPServer(t, cfg)
	defer server.Shutdown()

	// Make request through proxy
	resp, err := http.Get("http://" + proxyAddr)
	if err != nil {
		t.Fatalf("failed to make request through proxy: %v", err)
	}
	defer resp.Body.Close()

	// Verify response
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if string(body) != "Backend response" {
		t.Errorf("expected 'Backend response', got '%s'", string(body))
	}
}

func TestHttpProxyServer_ForwardedHeaders(t *testing.T) {
	// Create mock backend that echoes headers
	backend := mockHTTPBackend(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Forwarded-For-Echo", r.Header.Get("X-Forwarded-For"))
		w.Header().Set("X-Forwarded-Host-Echo", r.Header.Get("X-Forwarded-Host"))
		w.Header().Set("X-Forwarded-Proto-Echo", r.Header.Get("X-Forwarded-Proto"))
		w.Header().Set("X-Real-IP-Echo", r.Header.Get("X-Real-IP"))
		w.WriteHeader(http.StatusOK)
	})
	defer backend.Close()

	backendAddr := strings.TrimPrefix(backend.URL, "http://")
	cfg := createTestHTTPConfig("127.0.0.1:0", []string{backendAddr})
	server, proxyAddr := startTestHTTPServer(t, cfg)
	defer server.Shutdown()

	// Make request through proxy
	resp, err := http.Get("http://" + proxyAddr + "/test")
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Verify forwarded headers were set
	if resp.Header.Get("X-Forwarded-For-Echo") == "" {
		t.Error("X-Forwarded-For header was not set")
	}
	if resp.Header.Get("X-Forwarded-Host-Echo") == "" {
		t.Error("X-Forwarded-Host header was not set")
	}
	if resp.Header.Get("X-Forwarded-Proto-Echo") == "" {
		t.Error("X-Forwarded-Proto header was not set")
	}
	if resp.Header.Get("X-Real-IP-Echo") == "" {
		t.Error("X-Real-IP header was not set")
	}
}

func TestHttpProxyServer_MultipleBackends_RoundRobin(t *testing.T) {
	// Create two mock backends
	var backend1Hits, backend2Hits int
	var mu sync.Mutex

	backend1 := mockHTTPBackend(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		backend1Hits++
		mu.Unlock()
		_, _ = w.Write([]byte("backend1"))
	})
	defer backend1.Close()

	backend2 := mockHTTPBackend(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		backend2Hits++
		mu.Unlock()
		_, _ = w.Write([]byte("backend2"))
	})
	defer backend2.Close()

	backends := []string{
		strings.TrimPrefix(backend1.URL, "http://"),
		strings.TrimPrefix(backend2.URL, "http://"),
	}

	cfg := createTestHTTPConfig("127.0.0.1:0", backends)
	server, proxyAddr := startTestHTTPServer(t, cfg)
	defer server.Shutdown()

	// Make multiple requests
	numRequests := 10
	for i := 0; i < numRequests; i++ {
		resp, err := http.Get("http://" + proxyAddr)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		resp.Body.Close()
	}

	// Verify both backends received requests (round-robin)
	mu.Lock()
	defer mu.Unlock()

	if backend1Hits == 0 {
		t.Error("backend1 received no requests")
	}
	if backend2Hits == 0 {
		t.Error("backend2 received no requests")
	}

	// With round-robin, distribution should be relatively even
	if backend1Hits+backend2Hits != numRequests {
		t.Errorf("expected %d total requests, got %d", numRequests, backend1Hits+backend2Hits)
	}
}

func TestHttpProxyServer_PostRequest(t *testing.T) {
	// Create mock backend that echoes POST body
	backend := mockHTTPBackend(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read body: %v", err)
		}
		_, _ = w.Write(body)
	})
	defer backend.Close()

	backendAddr := strings.TrimPrefix(backend.URL, "http://")
	cfg := createTestHTTPConfig("127.0.0.1:0", []string{backendAddr})
	server, proxyAddr := startTestHTTPServer(t, cfg)
	defer server.Shutdown()

	// Make POST request
	testData := "test POST data"
	resp, err := http.Post("http://"+proxyAddr, "text/plain", strings.NewReader(testData))
	if err != nil {
		t.Fatalf("failed to make POST request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	if string(body) != testData {
		t.Errorf("expected '%s', got '%s'", testData, string(body))
	}
}

func TestHttpProxyServer_BackendError(t *testing.T) {
	// Create backend that returns error
	backend := mockHTTPBackend(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Backend error"))
	})
	defer backend.Close()

	backendAddr := strings.TrimPrefix(backend.URL, "http://")
	cfg := createTestHTTPConfig("127.0.0.1:0", []string{backendAddr})
	server, proxyAddr := startTestHTTPServer(t, cfg)
	defer server.Shutdown()

	// Make request
	resp, err := http.Get("http://" + proxyAddr)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Verify error status is passed through
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", resp.StatusCode)
	}
}

func TestHttpProxyServer_BackendUnavailable(t *testing.T) {
	// Create config with non-existent backend
	cfg := &conf.Config{
		Mode:   "http",
		Listen: "127.0.0.1:0",
		Nodes: []conf.Node{
			{Name: "node1", Address: "localhost:59999", Weight: 1}, // Non-existent
		},
		LoadBalancer: conf.LoadBalancerConfig{Algorithm: "round-robin"},
		Timeouts: conf.TimeoutConfig{
			Connect: 100 * time.Millisecond,
			Read:    1 * time.Second,
			Write:   1 * time.Second,
			Idle:    1 * time.Second,
		},
		HTTP: &conf.HTTPConfig{
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	server, proxyAddr := startTestHTTPServer(t, cfg)
	defer server.Shutdown()

	// Make request - should get bad gateway error
	resp, err := http.Get("http://" + proxyAddr)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected status 502 (Bad Gateway), got %d", resp.StatusCode)
	}
}

func TestHttpProxyServer_ConcurrentRequests(t *testing.T) {
	backend := mockHTTPBackend(t, func(w http.ResponseWriter, r *http.Request) {
		// Simulate some processing time
		time.Sleep(10 * time.Millisecond)
		_, _ = w.Write([]byte("OK"))
	})
	defer backend.Close()

	backendAddr := strings.TrimPrefix(backend.URL, "http://")
	cfg := createTestHTTPConfig("127.0.0.1:0", []string{backendAddr})
	server, proxyAddr := startTestHTTPServer(t, cfg)
	defer server.Shutdown()

	// Make concurrent requests
	numRequests := 50
	var wg sync.WaitGroup
	wg.Add(numRequests)
	errors := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(id int) {
			defer wg.Done()
			resp, err := http.Get("http://" + proxyAddr)
			if err != nil {
				errors <- fmt.Errorf("request %d failed: %v", id, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				errors <- fmt.Errorf("request %d: expected status 200, got %d", id, resp.StatusCode)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Error(err)
	}
}

func TestHttpProxyServer_PathPreservation(t *testing.T) {
	// Create backend that echoes the request path
	backend := mockHTTPBackend(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	})
	defer backend.Close()

	backendAddr := strings.TrimPrefix(backend.URL, "http://")
	cfg := createTestHTTPConfig("127.0.0.1:0", []string{backendAddr})
	server, proxyAddr := startTestHTTPServer(t, cfg)
	defer server.Shutdown()

	testPaths := []string{
		"/",
		"/api/users",
		"/api/users/123",
		"/api/users/123?foo=bar",
	}

	for _, path := range testPaths {
		resp, err := http.Get("http://" + proxyAddr + path)
		if err != nil {
			t.Fatalf("failed to request path %s: %v", path, err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatalf("failed to read response for path %s: %v", path, err)
		}

		// Extract just the path without query string for comparison
		expectedPath := strings.Split(path, "?")[0]
		if string(body) != expectedPath {
			t.Errorf("path %s: expected '%s', got '%s'", path, expectedPath, string(body))
		}
	}
}

func TestHttpProxyServer_QueryStringPreservation(t *testing.T) {
	// Create backend that echoes query parameters
	backend := mockHTTPBackend(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.RawQuery))
	})
	defer backend.Close()

	backendAddr := strings.TrimPrefix(backend.URL, "http://")
	cfg := createTestHTTPConfig("127.0.0.1:0", []string{backendAddr})
	server, proxyAddr := startTestHTTPServer(t, cfg)
	defer server.Shutdown()

	testQuery := "foo=bar&baz=qux&test=123"
	resp, err := http.Get("http://" + proxyAddr + "/test?" + testQuery)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	if string(body) != testQuery {
		t.Errorf("expected query '%s', got '%s'", testQuery, string(body))
	}
}

func TestHttpProxyServer_Shutdown(t *testing.T) {
	backend := mockHTTPBackend(t, nil)
	defer backend.Close()

	backendAddr := strings.TrimPrefix(backend.URL, "http://")
	cfg := createTestHTTPConfig("127.0.0.1:0", []string{backendAddr})
	server, proxyAddr := startTestHTTPServer(t, cfg)

	// Verify server is running
	resp, err := http.Get("http://" + proxyAddr)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	resp.Body.Close()

	// Shutdown server
	if err := server.Shutdown(); err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}

	// Verify server is shut down
	time.Sleep(200 * time.Millisecond)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	_, err = client.Get("http://" + proxyAddr)
	if err == nil {
		t.Error("expected request to fail after shutdown")
	}
}

func TestHttpProxyServer_Statistics(t *testing.T) {
	backend := mockHTTPBackend(t, nil)
	defer backend.Close()

	backendAddr := strings.TrimPrefix(backend.URL, "http://")
	cfg := createTestHTTPConfig("127.0.0.1:0", []string{backendAddr})
	server, proxyAddr := startTestHTTPServer(t, cfg)
	defer server.Shutdown()

	// Initial stats should be zero
	if server.stats.totalRequests.Load() != 0 {
		t.Error("totalRequests should start at 0")
	}

	// Make requests
	numRequests := 5
	for i := 0; i < numRequests; i++ {
		resp, err := http.Get("http://" + proxyAddr)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		_, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
	}

	// Verify stats
	time.Sleep(100 * time.Millisecond)
	totalReqs := server.stats.totalRequests.Load()
	if totalReqs < int64(numRequests) {
		t.Errorf("expected at least %d total requests, got %d", numRequests, totalReqs)
	}
}

func TestHttpProxyServer_CustomHeaders(t *testing.T) {
	// Create backend that echoes custom headers
	backend := mockHTTPBackend(t, func(w http.ResponseWriter, r *http.Request) {
		customHeader := r.Header.Get("X-Custom-Header")
		w.Header().Set("X-Custom-Response", customHeader)
		_, _ = w.Write([]byte("OK"))
	})
	defer backend.Close()

	backendAddr := strings.TrimPrefix(backend.URL, "http://")
	cfg := createTestHTTPConfig("127.0.0.1:0", []string{backendAddr})
	server, proxyAddr := startTestHTTPServer(t, cfg)
	defer server.Shutdown()

	// Make request with custom header
	client := &http.Client{}
	req, err := http.NewRequest("GET", "http://"+proxyAddr, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("X-Custom-Header", "test-value")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Verify custom header was preserved
	if resp.Header.Get("X-Custom-Response") != "test-value" {
		t.Errorf("expected custom header 'test-value', got '%s'", resp.Header.Get("X-Custom-Response"))
	}
}

// WebSocket test helpers and tests

// mockWebSocketBackend creates a simple WebSocket echo server
func mockWebSocketBackend(t *testing.T) net.Listener {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return // Listener closed
			}

			go handleWebSocketConnection(t, conn)
		}
	}()

	return listener
}

func handleWebSocketConnection(t *testing.T, conn net.Conn) {
	defer conn.Close()

	// Read the upgrade request
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		t.Logf("failed to read upgrade request: %v", err)
		return
	}

	request := string(buf[:n])
	if !strings.Contains(request, "Upgrade: websocket") {
		t.Logf("not a websocket upgrade request")
		return
	}

	// Send upgrade response
	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: test-accept-key\r\n" +
		"\r\n"

	if _, err := conn.Write([]byte(response)); err != nil {
		t.Logf("failed to write upgrade response: %v", err)
		return
	}

	// Echo messages back
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				t.Logf("error reading from connection: %v", err)
			}
			return
		}

		if _, err := conn.Write(buf[:n]); err != nil {
			t.Logf("error writing to connection: %v", err)
			return
		}
	}
}

func TestHttpProxyServer_WebSocket_BasicConnection(t *testing.T) {
	// Create WebSocket backend
	backend := mockWebSocketBackend(t)
	defer backend.Close()

	// Create config with WebSocket enabled
	cfg := createTestHTTPConfig("127.0.0.1:0", []string{backend.Addr().String()})
	cfg.HTTP.EnableWebSocket = true

	server, proxyAddr := startTestHTTPServer(t, cfg)
	defer server.Shutdown()

	// Create WebSocket upgrade request
	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer conn.Close()

	// Send upgrade request
	upgradeRequest := "GET / HTTP/1.1\r\n" +
		"Host: " + proxyAddr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: test-key\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"\r\n"

	if _, err := conn.Write([]byte(upgradeRequest)); err != nil {
		t.Fatalf("failed to write upgrade request: %v", err)
	}

	// Read upgrade response
	buf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("failed to read upgrade response: %v", err)
	}

	response := string(buf[:n])
	if !strings.Contains(response, "101 Switching Protocols") {
		t.Errorf("expected 101 Switching Protocols, got: %s", response)
	}
	if !strings.Contains(response, "Upgrade: websocket") {
		t.Error("expected Upgrade: websocket header in response")
	}
}

func TestHttpProxyServer_WebSocket_MessageEcho(t *testing.T) {
	// Create WebSocket backend
	backend := mockWebSocketBackend(t)
	defer backend.Close()

	// Create config with WebSocket enabled
	cfg := createTestHTTPConfig("127.0.0.1:0", []string{backend.Addr().String()})
	cfg.HTTP.EnableWebSocket = true

	server, proxyAddr := startTestHTTPServer(t, cfg)
	defer server.Shutdown()

	// Establish WebSocket connection
	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer conn.Close()

	// Send upgrade request
	upgradeRequest := "GET / HTTP/1.1\r\n" +
		"Host: " + proxyAddr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: test-key\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"\r\n"

	if _, err := conn.Write([]byte(upgradeRequest)); err != nil {
		t.Fatalf("failed to write upgrade request: %v", err)
	}

	// Read and discard upgrade response
	buf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.Read(buf)
	if err != nil {
		t.Fatalf("failed to read upgrade response: %v", err)
	}

	// Send test message
	testMessage := []byte("Hello WebSocket!")
	if _, err := conn.Write(testMessage); err != nil {
		t.Fatalf("failed to write test message: %v", err)
	}

	// Read echoed message
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("failed to read echoed message: %v", err)
	}

	if !strings.Contains(string(buf[:n]), string(testMessage)) {
		t.Errorf("expected echoed message '%s', got '%s'", testMessage, buf[:n])
	}
}

func TestHttpProxyServer_WebSocket_MultipleMessages(t *testing.T) {
	// Create WebSocket backend
	backend := mockWebSocketBackend(t)
	defer backend.Close()

	// Create config with WebSocket enabled
	cfg := createTestHTTPConfig("127.0.0.1:0", []string{backend.Addr().String()})
	cfg.HTTP.EnableWebSocket = true

	server, proxyAddr := startTestHTTPServer(t, cfg)
	defer server.Shutdown()

	// Establish WebSocket connection
	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer conn.Close()

	// Send upgrade request
	upgradeRequest := "GET / HTTP/1.1\r\n" +
		"Host: " + proxyAddr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: test-key\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"\r\n"

	if _, err := conn.Write([]byte(upgradeRequest)); err != nil {
		t.Fatalf("failed to write upgrade request: %v", err)
	}

	// Read upgrade response
	buf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.Read(buf)
	if err != nil {
		t.Fatalf("failed to read upgrade response: %v", err)
	}

	// Send multiple messages
	messages := []string{"message1", "message2", "message3"}
	for _, msg := range messages {
		if _, err := conn.Write([]byte(msg)); err != nil {
			t.Fatalf("failed to write message '%s': %v", msg, err)
		}

		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			t.Fatalf("failed to read echo for '%s': %v", msg, err)
		}

		if !strings.Contains(string(buf[:n]), msg) {
			t.Errorf("expected echo of '%s', got '%s'", msg, buf[:n])
		}
	}
}

func TestHttpProxyServer_WebSocket_DisabledByDefault(t *testing.T) {
	// Create a regular HTTP backend (not WebSocket)
	backend := mockHTTPBackend(t, func(w http.ResponseWriter, r *http.Request) {
		// If we get here, the proxy treated this as a regular HTTP request
		// which is correct when WebSocket is disabled
		if r.Header.Get("Upgrade") == "websocket" {
			// Respond with 400 to indicate we don't support WebSocket upgrade
			http.Error(w, "WebSocket not supported", http.StatusBadRequest)
		} else {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		}
	})
	defer backend.Close()

	backendAddr := strings.TrimPrefix(backend.URL, "http://")

	// Create config with WebSocket DISABLED
	cfg := createTestHTTPConfig("127.0.0.1:0", []string{backendAddr})
	cfg.HTTP.EnableWebSocket = false // Explicitly disabled

	server, proxyAddr := startTestHTTPServer(t, cfg)
	defer server.Shutdown()

	// Make a regular HTTP request with upgrade headers
	// When WebSocket is disabled, this should be treated as a regular HTTP request
	client := &http.Client{}
	req, err := http.NewRequest("GET", "http://"+proxyAddr, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// When WebSocket is disabled, the proxy should forward this as a regular HTTP request
	// The backend will respond with 400 because it doesn't support WebSocket
	// The key point is that we get an HTTP response, not a protocol upgrade
	if resp.StatusCode == 101 {
		t.Error("should not upgrade to WebSocket when disabled, expected HTTP response")
	}

	// Verify we got a regular HTTP response (not a hijacked connection)
	if resp.StatusCode == http.StatusBadRequest {
		// This is the expected behavior - the request was forwarded as regular HTTP
		// and the backend rejected the upgrade
		return
	}
}

func TestHttpProxyServer_WebSocket_BackendConnectionFailure(t *testing.T) {
	// Create config with non-existent backend
	cfg := &conf.Config{
		Mode:   "http",
		Listen: "127.0.0.1:0",
		Nodes: []conf.Node{
			{Name: "node1", Address: "localhost:59999", Weight: 1}, // Non-existent
		},
		LoadBalancer: conf.LoadBalancerConfig{Algorithm: "round-robin"},
		Timeouts: conf.TimeoutConfig{
			Connect: 100 * time.Millisecond,
			Read:    1 * time.Second,
			Write:   1 * time.Second,
			Idle:    1 * time.Second,
		},
		HTTP: &conf.HTTPConfig{
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			EnableWebSocket:     true,
		},
	}

	server, proxyAddr := startTestHTTPServer(t, cfg)
	defer server.Shutdown()

	// Try to establish WebSocket connection
	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to connect to proxy: %v", err)
	}
	defer conn.Close()

	// Send upgrade request
	upgradeRequest := "GET / HTTP/1.1\r\n" +
		"Host: " + proxyAddr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: test-key\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"\r\n"

	if _, err := conn.Write([]byte(upgradeRequest)); err != nil {
		t.Fatalf("failed to write upgrade request: %v", err)
	}

	// Should get error response
	buf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	response := string(buf[:n])
	if !strings.Contains(response, "502") && !strings.Contains(response, "Bad Gateway") {
		t.Errorf("expected 502 Bad Gateway error, got: %s", response)
	}
}

func TestHttpProxyServer_WebSocket_ConcurrentConnections(t *testing.T) {
	// Create WebSocket backend
	backend := mockWebSocketBackend(t)
	defer backend.Close()

	// Create config with WebSocket enabled
	cfg := createTestHTTPConfig("127.0.0.1:0", []string{backend.Addr().String()})
	cfg.HTTP.EnableWebSocket = true

	server, proxyAddr := startTestHTTPServer(t, cfg)
	defer server.Shutdown()

	// Create multiple concurrent WebSocket connections
	numConnections := 10
	var wg sync.WaitGroup
	wg.Add(numConnections)
	errors := make(chan error, numConnections)

	for i := 0; i < numConnections; i++ {
		go func(id int) {
			defer wg.Done()

			conn, err := net.Dial("tcp", proxyAddr)
			if err != nil {
				errors <- fmt.Errorf("connection %d: failed to dial: %v", id, err)
				return
			}
			defer conn.Close()

			// Send upgrade request
			upgradeRequest := "GET / HTTP/1.1\r\n" +
				"Host: " + proxyAddr + "\r\n" +
				"Upgrade: websocket\r\n" +
				"Connection: Upgrade\r\n" +
				"Sec-WebSocket-Key: test-key\r\n" +
				"Sec-WebSocket-Version: 13\r\n" +
				"\r\n"

			if _, err := conn.Write([]byte(upgradeRequest)); err != nil {
				errors <- fmt.Errorf("connection %d: failed to write upgrade: %v", id, err)
				return
			}

			// Read upgrade response
			buf := make([]byte, 4096)
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, err := conn.Read(buf)
			if err != nil {
				errors <- fmt.Errorf("connection %d: failed to read upgrade response: %v", id, err)
				return
			}

			if !strings.Contains(string(buf[:n]), "101 Switching Protocols") {
				errors <- fmt.Errorf("connection %d: expected 101 response", id)
				return
			}

			// Send and verify message
			testMsg := fmt.Sprintf("message from connection %d", id)
			if _, err := conn.Write([]byte(testMsg)); err != nil {
				errors <- fmt.Errorf("connection %d: failed to write message: %v", id, err)
				return
			}

			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, err = conn.Read(buf)
			if err != nil {
				errors <- fmt.Errorf("connection %d: failed to read echo: %v", id, err)
				return
			}

			if !strings.Contains(string(buf[:n]), testMsg) {
				errors <- fmt.Errorf("connection %d: echo mismatch", id)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Error(err)
	}
}

func TestHttpProxyServer_WebSocket_LoadBalancing(t *testing.T) {
	// Create two WebSocket backends
	backend1 := mockWebSocketBackend(t)
	defer backend1.Close()

	backend2 := mockWebSocketBackend(t)
	defer backend2.Close()

	// Create config with multiple backends
	backends := []string{backend1.Addr().String(), backend2.Addr().String()}
	cfg := createTestHTTPConfig("127.0.0.1:0", backends)
	cfg.HTTP.EnableWebSocket = true

	server, proxyAddr := startTestHTTPServer(t, cfg)
	defer server.Shutdown()

	// Create multiple connections to verify load balancing
	numConnections := 4
	successfulConnections := 0

	for i := 0; i < numConnections; i++ {
		conn, err := net.Dial("tcp", proxyAddr)
		if err != nil {
			t.Logf("connection %d failed to dial: %v", i, err)
			continue
		}

		// Send upgrade request
		upgradeRequest := "GET / HTTP/1.1\r\n" +
			"Host: " + proxyAddr + "\r\n" +
			"Upgrade: websocket\r\n" +
			"Connection: Upgrade\r\n" +
			"Sec-WebSocket-Key: test-key\r\n" +
			"Sec-WebSocket-Version: 13\r\n" +
			"\r\n"

		if _, err := conn.Write([]byte(upgradeRequest)); err != nil {
			conn.Close()
			t.Logf("connection %d failed to write upgrade: %v", i, err)
			continue
		}

		// Read upgrade response
		buf := make([]byte, 4096)
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			conn.Close()
			t.Logf("connection %d failed to read: %v", i, err)
			continue
		}

		if strings.Contains(string(buf[:n]), "101 Switching Protocols") {
			successfulConnections++
		}

		conn.Close()
	}

	// At least some connections should succeed
	if successfulConnections == 0 {
		t.Error("no WebSocket connections succeeded")
	}

	t.Logf("Successfully established %d/%d WebSocket connections through load balancer", successfulConnections, numConnections)
}

func TestIsWebSocketRequest(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		expected bool
	}{
		{
			name: "valid websocket upgrade",
			headers: map[string]string{
				"Upgrade":    "websocket",
				"Connection": "Upgrade",
			},
			expected: true,
		},
		{
			name: "valid websocket upgrade with mixed case",
			headers: map[string]string{
				"Upgrade":    "WebSocket",
				"Connection": "Upgrade",
			},
			expected: true,
		},
		{
			name: "missing upgrade header",
			headers: map[string]string{
				"Connection": "Upgrade",
			},
			expected: false,
		},
		{
			name: "missing connection header",
			headers: map[string]string{
				"Upgrade": "websocket",
			},
			expected: false,
		},
		{
			name: "wrong upgrade value",
			headers: map[string]string{
				"Upgrade":    "h2c",
				"Connection": "Upgrade",
			},
			expected: false,
		},
		{
			name: "connection header with multiple values",
			headers: map[string]string{
				"Upgrade":    "websocket",
				"Connection": "keep-alive, Upgrade",
			},
			expected: true,
		},
		{
			name:     "no headers",
			headers:  map[string]string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			result := isWebSocketRequest(req)
			if result != tt.expected {
				t.Errorf("isWebSocketRequest() = %v, expected %v", result, tt.expected)
			}
		})
	}
}
