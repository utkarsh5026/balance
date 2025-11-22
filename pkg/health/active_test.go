package health

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/utkarsh5026/balance/pkg/node"
)

// TestActiveCheckConfigDefaults tests that defaults are set correctly
func TestActiveCheckConfigDefaults(t *testing.T) {
	config := activeCheckConfig{}
	checker := NewActiveHealthChecker(config)

	if checker.networkType != CheckTypeTCP {
		t.Errorf("networkType = %v, want %v", checker.networkType, CheckTypeTCP)
	}
	if checker.timeout != 3*time.Second {
		t.Errorf("timeout = %v, want %v", checker.timeout, 3*time.Second)
	}
	if checker.httpPath != "/health" {
		t.Errorf("httpPath = %s, want %s", checker.httpPath, "/health")
	}
	if checker.httpMethod != http.MethodGet {
		t.Errorf("httpMethod = %s, want %s", checker.httpMethod, http.MethodGet)
	}
	if len(checker.expectedStatusCodes) != 1 || checker.expectedStatusCodes[0] != http.StatusOK {
		t.Errorf("expectedStatusCodes = %v, want [200]", checker.expectedStatusCodes)
	}
}

// TestActiveCheckerTCP tests TCP health checks
func TestActiveCheckerTCP(t *testing.T) {
	// Start a TCP listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().String()

	config := activeCheckConfig{
		NetworkType: CheckTypeTCP,
		Timeout:     2 * time.Second,
	}
	checker := NewActiveHealthChecker(config)

	n := node.NewNode("test", addr, 1)
	result := checker.Check(context.Background(), n)

	if !result.Success {
		t.Errorf("Check() success = %v, want %v, error: %v", result.Success, true, result.Error)
	}
	if result.Backend.Name() != "test" {
		t.Errorf("Backend name = %s, want %s", result.Backend.Name(), "test")
	}
	if result.Duration <= 0 {
		t.Error("Duration should be positive")
	}
}

// TestActiveCheckerTCPFailure tests TCP health check failure
func TestActiveCheckerTCPFailure(t *testing.T) {
	config := activeCheckConfig{
		NetworkType: CheckTypeTCP,
		Timeout:     1 * time.Second,
	}
	checker := NewActiveHealthChecker(config)

	// Use a port that's not listening
	n := node.NewNode("test", "127.0.0.1:9999", 1)
	result := checker.Check(context.Background(), n)

	if result.Success {
		t.Errorf("Check() success = %v, want %v", result.Success, false)
	}
	if result.Error == nil {
		t.Error("Error should not be nil for failed check")
	}
}

// TestActiveCheckerHTTP tests HTTP health checks
func TestActiveCheckerHTTP(t *testing.T) {
	// Start test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("Request path = %s, want %s", r.URL.Path, "/health")
		}
		if r.Method != http.MethodGet {
			t.Errorf("Request method = %s, want %s", r.Method, http.MethodGet)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Extract host:port from server URL
	addr := server.Listener.Addr().String()

	config := activeCheckConfig{
		NetworkType:         CheckTypeHTTP,
		Timeout:             2 * time.Second,
		HTTPPath:            "/health",
		ExpectedStatusCodes: []int{http.StatusOK},
	}
	checker := NewActiveHealthChecker(config)

	n := node.NewNode("test", addr, 1)
	result := checker.Check(context.Background(), n)

	if !result.Success {
		t.Errorf("Check() success = %v, want %v, error: %v", result.Success, true, result.Error)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", result.StatusCode, http.StatusOK)
	}
}

// TestActiveCheckerHTTPCustomMethod tests HTTP health checks with custom method
func TestActiveCheckerHTTPCustomMethod(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("Request method = %s, want %s", r.Method, http.MethodHead)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	addr := server.Listener.Addr().String()

	config := activeCheckConfig{
		NetworkType: CheckTypeHTTP,
		Timeout:     2 * time.Second,
		HTTPPath:    "/health",
		HTTPMethod:  http.MethodHead,
	}
	checker := NewActiveHealthChecker(config)

	n := node.NewNode("test", addr, 1)
	result := checker.Check(context.Background(), n)

	if !result.Success {
		t.Errorf("Check() success = %v, want %v, error: %v", result.Success, true, result.Error)
	}
}

// TestActiveCheckerHTTPUnexpectedStatus tests HTTP health check with unexpected status code
func TestActiveCheckerHTTPUnexpectedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	addr := server.Listener.Addr().String()

	config := activeCheckConfig{
		NetworkType:         CheckTypeHTTP,
		Timeout:             2 * time.Second,
		HTTPPath:            "/health",
		ExpectedStatusCodes: []int{http.StatusOK},
	}
	checker := NewActiveHealthChecker(config)

	n := node.NewNode("test", addr, 1)
	result := checker.Check(context.Background(), n)

	if result.Success {
		t.Errorf("Check() success = %v, want %v", result.Success, false)
	}
	if result.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("StatusCode = %d, want %d", result.StatusCode, http.StatusServiceUnavailable)
	}
	if result.Error == nil {
		t.Error("Error should not be nil for unexpected status")
	}
}

// TestActiveCheckerHTTPMultipleExpectedCodes tests HTTP health check with multiple expected codes
func TestActiveCheckerHTTPMultipleExpectedCodes(t *testing.T) {
	testCases := []int{http.StatusOK, http.StatusAccepted, http.StatusNoContent}

	for _, expectedCode := range testCases {
		t.Run(http.StatusText(expectedCode), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(expectedCode)
			}))
			defer server.Close()

			addr := server.Listener.Addr().String()

			config := activeCheckConfig{
				NetworkType:         CheckTypeHTTP,
				Timeout:             2 * time.Second,
				HTTPPath:            "/health",
				ExpectedStatusCodes: []int{http.StatusOK, http.StatusAccepted, http.StatusNoContent},
			}
			checker := NewActiveHealthChecker(config)

			n := node.NewNode("test", addr, 1)
			result := checker.Check(context.Background(), n)

			if !result.Success {
				t.Errorf("Check() success = %v, want %v, error: %v", result.Success, true, result.Error)
			}
			if result.StatusCode != expectedCode {
				t.Errorf("StatusCode = %d, want %d", result.StatusCode, expectedCode)
			}
		})
	}
}

// TestActiveCheckerContextTimeout tests that context timeout is respected
func TestActiveCheckerContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Delay longer than context timeout
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	addr := server.Listener.Addr().String()

	config := activeCheckConfig{
		NetworkType: CheckTypeHTTP,
		Timeout:     100 * time.Millisecond,
		HTTPPath:    "/health",
	}
	checker := NewActiveHealthChecker(config)

	n := node.NewNode("test", addr, 1)
	result := checker.Check(context.Background(), n)

	if result.Success {
		t.Errorf("Check() success = %v, want %v", result.Success, false)
	}
	if result.Error == nil {
		t.Error("Error should not be nil for timeout")
	}
}

// TestActiveCheckerCheckMultiple tests concurrent health checks
func TestActiveCheckerCheckMultiple(t *testing.T) {
	// Start multiple test servers
	servers := make([]*httptest.Server, 3)
	nodes := make([]*node.Node, 3)

	for i := 0; i < 3; i++ {
		idx := i
		servers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if idx == 1 {
				// Make second server return error
				w.WriteHeader(http.StatusServiceUnavailable)
			} else {
				w.WriteHeader(http.StatusOK)
			}
		}))
		defer servers[i].Close()

		addr := servers[i].Listener.Addr().String()
		nodes[i] = node.NewNode("backend"+string(rune('0'+i)), addr, 1)
	}

	config := activeCheckConfig{
		NetworkType: CheckTypeHTTP,
		Timeout:     2 * time.Second,
		HTTPPath:    "/health",
	}
	checker := NewActiveHealthChecker(config)

	results, err := checker.CheckMultiple(context.Background(), nodes)
	if err != nil {
		t.Fatalf("CheckMultiple() error = %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want %d", len(results), 3)
	}

	// First and third should succeed
	if !results[0].Success {
		t.Errorf("results[0].Success = %v, want %v", results[0].Success, true)
	}
	if !results[2].Success {
		t.Errorf("results[2].Success = %v, want %v", results[2].Success, true)
	}

	// Second should fail
	if results[1].Success {
		t.Errorf("results[1].Success = %v, want %v", results[1].Success, false)
	}
}

// TestActiveCheckerCheckMultipleWithCancel tests that CheckMultiple respects context cancellation
func TestActiveCheckerCheckMultipleWithCancel(t *testing.T) {
	// Create servers that will delay
	servers := make([]*httptest.Server, 2)
	nodes := make([]*node.Node, 2)

	for i := 0; i < 2; i++ {
		servers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(500 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer servers[i].Close()

		addr := servers[i].Listener.Addr().String()
		nodes[i] = node.NewNode("backend"+string(rune('0'+i)), addr, 1)
	}

	config := activeCheckConfig{
		NetworkType: CheckTypeHTTP,
		Timeout:     1 * time.Second,
		HTTPPath:    "/health",
	}
	checker := NewActiveHealthChecker(config)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	results, err := checker.CheckMultiple(ctx, nodes)
	if err != nil {
		t.Fatalf("CheckMultiple() error = %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want %d", len(results), 2)
	}

	// All should fail due to context cancellation
	for i, result := range results {
		if result.Success {
			t.Errorf("results[%d].Success = %v, want %v", i, result.Success, false)
		}
	}
}

// TestActiveCheckerClose tests cleanup
func TestActiveCheckerClose(t *testing.T) {
	config := activeCheckConfig{
		NetworkType: CheckTypeHTTP,
		Timeout:     2 * time.Second,
	}
	checker := NewActiveHealthChecker(config)

	// Should not panic
	checker.Close()

	// Client should be closed
	if checker.client == nil {
		t.Error("client should not be nil after close")
	}
}

// TestActiveCheckerUserAgent tests that user agent is set
func TestActiveCheckerUserAgent(t *testing.T) {
	var receivedUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUserAgent = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	addr := server.Listener.Addr().String()

	config := activeCheckConfig{
		NetworkType: CheckTypeHTTP,
		Timeout:     2 * time.Second,
		HTTPPath:    "/health",
	}
	checker := NewActiveHealthChecker(config)

	n := node.NewNode("test", addr, 1)
	checker.Check(context.Background(), n)

	if receivedUserAgent != "balance-health-checker/1.0" {
		t.Errorf("User-Agent = %s, want %s", receivedUserAgent, "balance-health-checker/1.0")
	}
}
