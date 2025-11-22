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
	"github.com/utkarsh5026/poolme/pool"
)

type NetworkType string

const (
	CheckTypeTCP   NetworkType = "tcp"
	CheckTypeHTTP  NetworkType = "http"
	CheckTypeHTTPS NetworkType = "https"
)

// activeCheckConfig configures an active health checker
type activeCheckConfig struct {
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

type activeCheckResult struct {
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

type activeChecker struct {
	networkType NetworkType

	timeout time.Duration

	httpPath string

	httpMethod string

	expectedStatusCodes []int

	client *http.Client
}

func NewActiveHealthChecker(config activeCheckConfig) *activeChecker {
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

	return &activeChecker{
		networkType:         config.NetworkType,
		timeout:             config.Timeout,
		httpPath:            config.HTTPPath,
		httpMethod:          config.HTTPMethod,
		expectedStatusCodes: config.ExpectedStatusCodes,
		client:              httpClient,
	}
}

func (ac *activeChecker) Check(ctx context.Context, n *node.Node) activeCheckResult {
	start := time.Now()
	result := activeCheckResult{
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
func (ac *activeChecker) checkTCP(ctx context.Context, address string) error {
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

func (ac *activeChecker) checkHTTP(ctx context.Context, url string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, ac.httpMethod, url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "balance-health-checker/1.0")
	resp, err := ac.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	_, _ = io.Copy(io.Discard, resp.Body)
	if slices.Contains(ac.expectedStatusCodes, resp.StatusCode) {
		return resp.StatusCode, nil
	}

	return resp.StatusCode, fmt.Errorf("unexpected status code: %d (expected: %v)", resp.StatusCode, ac.expectedStatusCodes)
}

func (ac *activeChecker) CheckMultiple(ctx context.Context, backends []*node.Node) ([]activeCheckResult, error) {
	p := pool.NewWorkerPool[*node.Node, activeCheckResult]()

	results, err := p.Process(ctx, backends, func(ctx context.Context, backend *node.Node) (activeCheckResult, error) {
		result := ac.Check(ctx, backend)
		return result, nil
	})

	// Context cancellation is expected behavior in health checking
	// Return the partial results without treating it as an error
	if err != nil && ctx.Err() == nil {
		return results, fmt.Errorf("pool processing error: %w", err)
	}

	return results, nil
}

// Close cleans up resources used by the health checker
func (ac *activeChecker) Close() {
	if ac.client != nil {
		ac.client.CloseIdleConnections()
	}
}
