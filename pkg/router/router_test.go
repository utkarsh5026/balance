package router

import (
	"net/http/httptest"
	"testing"

	"github.com/utkarsh5026/balance/pkg/conf"
	"github.com/utkarsh5026/balance/pkg/node"
)

func createTestNodes() []*node.Node {
	return []*node.Node{
		node.NewNode("backend1", "localhost:8001", 1),
		node.NewNode("backend2", "localhost:8002", 1),
		node.NewNode("backend3", "localhost:8003", 1),
		node.NewNode("api-backend1", "localhost:9001", 1),
		node.NewNode("api-backend2", "localhost:9002", 1),
	}
}

func createDefaultPool(nodes []*node.Node) *node.Pool {
	pool := node.NewPool()
	for _, n := range nodes {
		pool.Add(n)
	}
	return pool
}

func TestNewRouter(t *testing.T) {
	nodes := createTestNodes()
	defaultPool := createDefaultPool(nodes)

	routes := []conf.Route{
		{
			Name:       "api-route",
			PathPrefix: "/api",
			Backends:   []string{"api-backend1", "api-backend2"},
			Priority:   1,
		},
		{
			Name:       "default-route",
			PathPrefix: "/",
			Backends:   []string{"backend1", "backend2"},
			Priority:   10,
		},
	}

	router := NewRouter(routes, defaultPool)

	if router == nil {
		t.Fatal("NewRouter returned nil")
	}

	if router.defaultPool != defaultPool {
		t.Error("Default pool not set correctly")
	}

	if len(router.routes) != 2 {
		t.Errorf("Expected 2 routes, got %d", len(router.routes))
	}

	// Routes should be sorted by priority (lower priority first)
	if router.routes[0].config.Priority != 1 {
		t.Errorf("Expected first route priority 1, got %d", router.routes[0].config.Priority)
	}
	if router.routes[1].config.Priority != 10 {
		t.Errorf("Expected second route priority 10, got %d", router.routes[1].config.Priority)
	}
}

func TestNewRouterPoolCreation(t *testing.T) {
	nodes := createTestNodes()
	defaultPool := createDefaultPool(nodes)

	routes := []conf.Route{
		{
			Name:     "api-route",
			Backends: []string{"api-backend1", "api-backend2"},
			Priority: 1,
		},
	}

	router := NewRouter(routes, defaultPool)

	if router.routes[0].pool.Count() != 2 {
		t.Errorf("Expected route pool to have 2 backends, got %d", router.routes[0].pool.Count())
	}

	backend1 := router.routes[0].pool.GetByName("api-backend1")
	if backend1 == nil {
		t.Error("api-backend1 not found in route pool")
	}

	backend2 := router.routes[0].pool.GetByName("api-backend2")
	if backend2 == nil {
		t.Error("api-backend2 not found in route pool")
	}
}

func TestNewRouterNonExistentBackend(t *testing.T) {
	nodes := createTestNodes()
	defaultPool := createDefaultPool(nodes)

	routes := []conf.Route{
		{
			Name:     "route-with-missing-backend",
			Backends: []string{"nonexistent-backend", "api-backend1"},
			Priority: 1,
		},
	}

	router := NewRouter(routes, defaultPool)

	// Should only add backends that exist
	if router.routes[0].pool.Count() != 1 {
		t.Errorf("Expected route pool to have 1 backend (ignoring nonexistent), got %d", router.routes[0].pool.Count())
	}
}

func TestMatchPathPrefix(t *testing.T) {
	nodes := createTestNodes()
	defaultPool := createDefaultPool(nodes)

	routes := []conf.Route{
		{
			Name:       "api-route",
			PathPrefix: "/api",
			Backends:   []string{"api-backend1", "api-backend2"},
			Priority:   1,
		},
	}

	router := NewRouter(routes, defaultPool)

	tests := []struct {
		name          string
		path          string
		shouldMatch   bool
		expectedPool  *node.Pool
	}{
		{
			name:         "exact api prefix",
			path:         "/api",
			shouldMatch:  true,
			expectedPool: router.routes[0].pool,
		},
		{
			name:         "api with sub-path",
			path:         "/api/users",
			shouldMatch:  true,
			expectedPool: router.routes[0].pool,
		},
		{
			name:         "api with nested path",
			path:         "/api/v1/users",
			shouldMatch:  true,
			expectedPool: router.routes[0].pool,
		},
		{
			name:         "non-matching path",
			path:         "/other",
			shouldMatch:  false,
			expectedPool: defaultPool,
		},
		{
			name:         "root path",
			path:         "/",
			shouldMatch:  false,
			expectedPool: defaultPool,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			matchedPool := router.Match(req)

			if matchedPool != tt.expectedPool {
				t.Errorf("Expected pool %v, got %v", tt.expectedPool, matchedPool)
			}
		})
	}
}

func TestMatchHost(t *testing.T) {
	nodes := createTestNodes()
	defaultPool := createDefaultPool(nodes)

	routes := []conf.Route{
		{
			Name:     "api-host-route",
			Host:     "api.example.com",
			Backends: []string{"api-backend1"},
			Priority: 1,
		},
	}

	router := NewRouter(routes, defaultPool)

	tests := []struct {
		name          string
		host          string
		expectedPool  *node.Pool
	}{
		{
			name:         "exact host match",
			host:         "api.example.com",
			expectedPool: router.routes[0].pool,
		},
		{
			name:         "host with port",
			host:         "api.example.com:8080",
			expectedPool: router.routes[0].pool,
		},
		{
			name:         "non-matching host",
			host:         "web.example.com",
			expectedPool: defaultPool,
		},
		{
			name:         "different domain",
			host:         "example.org",
			expectedPool: defaultPool,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.Host = tt.host
			matchedPool := router.Match(req)

			if matchedPool != tt.expectedPool {
				t.Errorf("Expected pool %v, got %v", tt.expectedPool, matchedPool)
			}
		})
	}
}

func TestMatchWildcardHost(t *testing.T) {
	nodes := createTestNodes()
	defaultPool := createDefaultPool(nodes)

	routes := []conf.Route{
		{
			Name:     "wildcard-host-route",
			Host:     "*.example.com",
			Backends: []string{"backend1"},
			Priority: 1,
		},
	}

	router := NewRouter(routes, defaultPool)

	tests := []struct {
		name          string
		host          string
		expectedPool  *node.Pool
	}{
		{
			name:         "subdomain match",
			host:         "api.example.com",
			expectedPool: router.routes[0].pool,
		},
		{
			name:         "another subdomain",
			host:         "web.example.com",
			expectedPool: router.routes[0].pool,
		},
		{
			name:         "nested subdomain",
			host:         "api.v1.example.com",
			expectedPool: router.routes[0].pool,
		},
		{
			name:         "with port",
			host:         "api.example.com:8080",
			expectedPool: router.routes[0].pool,
		},
		{
			name:         "different domain",
			host:         "example.org",
			expectedPool: defaultPool,
		},
		{
			name:         "root domain should not match",
			host:         "example.com",
			expectedPool: defaultPool,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.Host = tt.host
			matchedPool := router.Match(req)

			if matchedPool != tt.expectedPool {
				t.Errorf("Expected pool %v, got %v", tt.expectedPool, matchedPool)
			}
		})
	}
}

func TestMatchHeaders(t *testing.T) {
	nodes := createTestNodes()
	defaultPool := createDefaultPool(nodes)

	routes := []conf.Route{
		{
			Name: "header-route",
			Headers: map[string]string{
				"X-API-Key": "secret123",
			},
			Backends: []string{"api-backend1"},
			Priority: 1,
		},
	}

	router := NewRouter(routes, defaultPool)

	tests := []struct {
		name          string
		headers       map[string]string
		expectedPool  *node.Pool
	}{
		{
			name: "matching header",
			headers: map[string]string{
				"X-API-Key": "secret123",
			},
			expectedPool: router.routes[0].pool,
		},
		{
			name: "matching header with extra headers",
			headers: map[string]string{
				"X-API-Key":  "secret123",
				"User-Agent": "test",
			},
			expectedPool: router.routes[0].pool,
		},
		{
			name: "wrong header value",
			headers: map[string]string{
				"X-API-Key": "wrong-key",
			},
			expectedPool: defaultPool,
		},
		{
			name:         "missing header",
			headers:      map[string]string{},
			expectedPool: defaultPool,
		},
		{
			name: "different header",
			headers: map[string]string{
				"Authorization": "Bearer token",
			},
			expectedPool: defaultPool,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			matchedPool := router.Match(req)

			if matchedPool != tt.expectedPool {
				t.Errorf("Expected pool %v, got %v", tt.expectedPool, matchedPool)
			}
		})
	}
}

func TestMatchMultipleHeaders(t *testing.T) {
	nodes := createTestNodes()
	defaultPool := createDefaultPool(nodes)

	routes := []conf.Route{
		{
			Name: "multi-header-route",
			Headers: map[string]string{
				"X-API-Key":     "secret123",
				"X-API-Version": "v1",
			},
			Backends: []string{"api-backend1"},
			Priority: 1,
		},
	}

	router := NewRouter(routes, defaultPool)

	tests := []struct {
		name          string
		headers       map[string]string
		expectedPool  *node.Pool
	}{
		{
			name: "all headers match",
			headers: map[string]string{
				"X-API-Key":     "secret123",
				"X-API-Version": "v1",
			},
			expectedPool: router.routes[0].pool,
		},
		{
			name: "only one header matches",
			headers: map[string]string{
				"X-API-Key": "secret123",
			},
			expectedPool: defaultPool,
		},
		{
			name: "one header wrong value",
			headers: map[string]string{
				"X-API-Key":     "secret123",
				"X-API-Version": "v2",
			},
			expectedPool: defaultPool,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			matchedPool := router.Match(req)

			if matchedPool != tt.expectedPool {
				t.Errorf("Expected pool %v, got %v", tt.expectedPool, matchedPool)
			}
		})
	}
}

func TestMatchCombinedConditions(t *testing.T) {
	nodes := createTestNodes()
	defaultPool := createDefaultPool(nodes)

	routes := []conf.Route{
		{
			Name:       "combined-route",
			Host:       "api.example.com",
			PathPrefix: "/v1",
			Headers: map[string]string{
				"X-API-Key": "secret123",
			},
			Backends: []string{"api-backend1"},
			Priority: 1,
		},
	}

	router := NewRouter(routes, defaultPool)

	tests := []struct {
		name          string
		host          string
		path          string
		headers       map[string]string
		expectedPool  *node.Pool
	}{
		{
			name: "all conditions match",
			host: "api.example.com",
			path: "/v1/users",
			headers: map[string]string{
				"X-API-Key": "secret123",
			},
			expectedPool: router.routes[0].pool,
		},
		{
			name: "wrong host",
			host: "web.example.com",
			path: "/v1/users",
			headers: map[string]string{
				"X-API-Key": "secret123",
			},
			expectedPool: defaultPool,
		},
		{
			name: "wrong path",
			host: "api.example.com",
			path: "/v2/users",
			headers: map[string]string{
				"X-API-Key": "secret123",
			},
			expectedPool: defaultPool,
		},
		{
			name:    "wrong header",
			host:    "api.example.com",
			path:    "/v1/users",
			headers: map[string]string{},
			expectedPool: defaultPool,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			req.Host = tt.host
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			matchedPool := router.Match(req)

			if matchedPool != tt.expectedPool {
				t.Errorf("Expected pool %v, got %v", tt.expectedPool, matchedPool)
			}
		})
	}
}

func TestRoutePriority(t *testing.T) {
	nodes := createTestNodes()
	defaultPool := createDefaultPool(nodes)

	routes := []conf.Route{
		{
			Name:       "low-priority",
			PathPrefix: "/api",
			Backends:   []string{"backend1"},
			Priority:   10,
		},
		{
			Name:       "high-priority",
			PathPrefix: "/api",
			Backends:   []string{"api-backend1"},
			Priority:   1,
		},
	}

	router := NewRouter(routes, defaultPool)

	// Both routes match /api, but high-priority should be checked first
	req := httptest.NewRequest("GET", "/api/test", nil)
	matchedPool := router.Match(req)

	// Should match the high-priority route (priority 1)
	if matchedPool != router.routes[0].pool {
		t.Error("Did not match high-priority route")
	}

	backend := matchedPool.GetByName("api-backend1")
	if backend == nil {
		t.Error("Expected api-backend1 from high-priority route")
	}
}

func TestMatchEmptyRoute(t *testing.T) {
	nodes := createTestNodes()
	defaultPool := createDefaultPool(nodes)

	routes := []conf.Route{
		{
			Name:     "catch-all",
			Backends: []string{"backend1"},
			Priority: 10,
		},
	}

	router := NewRouter(routes, defaultPool)

	// Route with no conditions should match everything
	req := httptest.NewRequest("GET", "/anything", nil)
	matchedPool := router.Match(req)

	if matchedPool != router.routes[0].pool {
		t.Error("Empty route should match all requests")
	}
}

func TestMatchNoRoutes(t *testing.T) {
	nodes := createTestNodes()
	defaultPool := createDefaultPool(nodes)

	router := NewRouter([]conf.Route{}, defaultPool)

	req := httptest.NewRequest("GET", "/test", nil)
	matchedPool := router.Match(req)

	if matchedPool != defaultPool {
		t.Error("Should return default pool when no routes defined")
	}
}

func TestMatchHostFunction(t *testing.T) {
	tests := []struct {
		name      string
		reqHost   string
		routeHost string
		expected  bool
	}{
		{
			name:      "exact match",
			reqHost:   "api.example.com",
			routeHost: "api.example.com",
			expected:  true,
		},
		{
			name:      "exact match with port",
			reqHost:   "api.example.com:8080",
			routeHost: "api.example.com",
			expected:  true,
		},
		{
			name:      "wildcard subdomain match",
			reqHost:   "api.example.com",
			routeHost: "*.example.com",
			expected:  true,
		},
		{
			name:      "wildcard with port",
			reqHost:   "api.example.com:8080",
			routeHost: "*.example.com",
			expected:  true,
		},
		{
			name:      "wildcard different domain",
			reqHost:   "api.different.com",
			routeHost: "*.example.com",
			expected:  false,
		},
		{
			name:      "exact mismatch",
			reqHost:   "web.example.com",
			routeHost: "api.example.com",
			expected:  false,
		},
		{
			name:      "base domain vs wildcard",
			reqHost:   "example.com",
			routeHost: "*.example.com",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchHost(tt.reqHost, tt.routeHost)
			if result != tt.expected {
				t.Errorf("matchHost(%q, %q) = %v, want %v", tt.reqHost, tt.routeHost, result, tt.expected)
			}
		})
	}
}

func TestMatchDifferentHTTPMethods(t *testing.T) {
	nodes := createTestNodes()
	defaultPool := createDefaultPool(nodes)

	routes := []conf.Route{
		{
			Name:       "api-route",
			PathPrefix: "/api",
			Backends:   []string{"api-backend1"},
			Priority:   1,
		},
	}

	router := NewRouter(routes, defaultPool)

	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/test", nil)
			matchedPool := router.Match(req)

			if matchedPool != router.routes[0].pool {
				t.Errorf("Route should match for %s method", method)
			}
		})
	}
}
