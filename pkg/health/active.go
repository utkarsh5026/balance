package health

import (
	"context"
	"fmt"
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
	CheckTypeGRPC  NetworkType = "grpc"
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

	httpClient := &http.Client{
		Timeout: config.Timeout,
		Transport: &http.Transport{
			DisableKeepAlives:   true,
			MaxIdleConnsPerHost: 1,
			IdleConnTimeout:     config.Timeout,
		},
	}

	return &ActiveHealthChecker{
		networkType:         config.NetworkType,
		timeout:             config.Timeout,
		httpPath:            config.HTTPPath,
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
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("TCP connection failed: %w", err)
	}
	if err := conn.Close(); err != nil {
		return fmt.Errorf("error closing tcp connection")
	}

	return nil
}

func (ac *ActiveHealthChecker) checkHTTP(ctx context.Context, url string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := ac.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if slices.Contains(ac.expectedStatusCodes, resp.StatusCode) {
		return resp.StatusCode, nil
	}

	return resp.StatusCode, fmt.Errorf("unexpected status code: %d (expected: %v)", resp.StatusCode, ac.expectedStatusCodes)
}
