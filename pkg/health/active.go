package health

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"time"

	"github.com/utkarsh5026/balance/pkg/node"
)

type NetworkType string

const (
	CheckTypeTCP   NetworkType = "tcp"
	CheckTypeHTTP  NetworkType = "http"
	CheckTypeHTTPS NetworkType = "https"
)

// ActiveCheckerConfig configures an active health checker
type ActiveCheckerConfig struct {
	// NetworkType specifies the type of health check
	NetworkType NetworkType

	// Timeout for each health check
	Timeout time.Duration

	// HTTPPath is the path for HTTP health checks (e.g., "/health")
	HTTPPath string

	// ExpectedStatusCodes are the HTTP status codes considered healthy
	ExpectedStatusCodes []int

	// HTTPMethod is the HTTP method to use (defaults to GET)
	HTTPMethod string

	// MaxIdleConns controls the maximum number of idle connections (defaults to 10)
	MaxIdleConns int

	// EnableKeepAlive enables HTTP keep-alive connections (defaults to false)
	EnableKeepAlive bool
}

type ActiveCheckResult struct {
	Backend *node.Node
	// Success indicates if the check passed
	Success bool

	// Error contains any error that occurred
	Error error

	// Duration of the health check
	Duration time.Duration

	// Timestamp of the check
	Timestamp time.Time

	// StatusCode for HTTP checks
	StatusCode int
}

type ActiveHealthChecker struct {
	networkType NetworkType

	timeout time.Duration

	httpPath string

	httpMethod string

	expectedStatusCodes []int

	client *http.Client
}

func NewActiveHealthChecker(config ActiveCheckerConfig) *ActiveHealthChecker {
	if config.NetworkType == "" {
		config.NetworkType = CheckTypeTCP
	}
	if config.Timeout == 0 {
		config.Timeout = 3 * time.Second
	}
	if config.HTTPPath == "" {
		config.HTTPPath = "/health"
	}
	if len(config.ExpectedStatusCodes) == 0 {
		config.ExpectedStatusCodes = []int{http.StatusOK}
	}
	if config.HTTPMethod == "" {
		config.HTTPMethod = http.MethodGet
	}
	if config.MaxIdleConns == 0 {
		config.MaxIdleConns = 10
	}

	httpClient := &http.Client{
		Timeout: config.Timeout,
		Transport: &http.Transport{
			DisableKeepAlives:   !config.EnableKeepAlive,
			MaxIdleConnsPerHost: config.MaxIdleConns,
			IdleConnTimeout:     config.Timeout,
			// Add reasonable connection pool settings
			MaxIdleConns:       config.MaxIdleConns * 2,
			DisableCompression: true,
		},
	}

	return &ActiveHealthChecker{
		networkType:         config.NetworkType,
		timeout:             config.Timeout,
		httpPath:            config.HTTPPath,
		httpMethod:          config.HTTPMethod,
		expectedStatusCodes: config.ExpectedStatusCodes,
		client:              httpClient,
	}
}

func (ac *ActiveHealthChecker) Check(ctx context.Context, n *node.Node) ActiveCheckResult {
	start := time.Now()
	result := ActiveCheckResult{
		Backend:   n,
		Timestamp: start,
	}

	var err error

	switch ac.networkType {
	case CheckTypeTCP:
		err = ac.checkTCP(ctx, n.Address())
	case CheckTypeHTTP:
		result.StatusCode, err = ac.checkHTTP(ctx, fmt.Sprintf("http://%s%s", n.Address(), ac.httpPath))
	case CheckTypeHTTPS:
		result.StatusCode, err = ac.checkHTTP(ctx, fmt.Sprintf("https://%s%s", n.Address(), ac.httpPath))
	default:
		err = fmt.Errorf("unsupported check type: %s", ac.networkType)
	}

	result.Duration = time.Since(start)
	result.Error = err
	result.Success = err == nil
	return result
}

// checkTCP performs a TCP connection check
func (ac *ActiveHealthChecker) checkTCP(ctx context.Context, address string) error {
	dialer := &net.Dialer{
		Timeout: ac.timeout,
	}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("TCP connection failed: %w", err)
	}
	if err := conn.Close(); err != nil {
		return fmt.Errorf("error closing tcp connection: %w", err)
	}

	return nil
}

func (ac *ActiveHealthChecker) checkHTTP(ctx context.Context, url string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, ac.httpMethod, url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	// Set User-Agent to identify health check requests
	req.Header.Set("User-Agent", "balance-health-checker/1.0")

	resp, err := ac.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Drain and discard body to allow connection reuse
	_, _ = io.Copy(io.Discard, resp.Body)

	if slices.Contains(ac.expectedStatusCodes, resp.StatusCode) {
		return resp.StatusCode, nil
	}

	return resp.StatusCode, fmt.Errorf("unexpected status code: %d (expected: %v)", resp.StatusCode, ac.expectedStatusCodes)
}

func (ac *ActiveHealthChecker) CheckMultiple(ctx context.Context, backends []*node.Node) []ActiveCheckResult {
	results := make([]ActiveCheckResult, len(backends))
	resultChan := make(chan struct {
		index  int
		result ActiveCheckResult
	}, len(backends))

	for i, b := range backends {
		go func(index int, backend *node.Node) {
			select {
			case <-ctx.Done():
				resultChan <- struct {
					index  int
					result ActiveCheckResult
				}{
					index: index,
					result: ActiveCheckResult{
						Backend:   backend,
						Success:   false,
						Error:     ctx.Err(),
						Duration:  0,
						Timestamp: time.Now(),
					},
				}
			default:
				result := ac.Check(ctx, backend)
				resultChan <- struct {
					index  int
					result ActiveCheckResult
				}{index, result}
			}
		}(i, b)
	}

	for range backends {
		select {
		case res := <-resultChan:
			results[res.index] = res.result
		case <-ctx.Done():
			return results
		}
	}

	return results
}

// Close cleans up resources used by the health checker
func (ac *ActiveHealthChecker) Close() {
	if ac.client != nil {
		ac.client.CloseIdleConnections()
	}
}
