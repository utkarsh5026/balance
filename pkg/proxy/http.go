package proxy

import (
	"context"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/utkarsh5026/balance/pkg/balance"
	"github.com/utkarsh5026/balance/pkg/conf"
	"github.com/utkarsh5026/balance/pkg/node"
	"github.com/utkarsh5026/balance/pkg/router"
	"github.com/utkarsh5026/balance/pkg/security"
	"github.com/utkarsh5026/balance/pkg/transport"
	"golang.org/x/net/http2"
	"golang.org/x/sync/errgroup"
)

const (
	MaxHeaderBytes = 1 << 20 // 1 MB
)

type HttpProxyServer struct {
	config          *conf.Config
	server          *http.Server
	pool            *node.Pool
	balancer        balance.LoadBalancer
	router          *router.Router
	transport       *http.Transport
	terminator      *transport.Terminator
	securityManager *security.SecurityManager

	ctx        context.Context
	cancelFunc context.CancelFunc
	stats      *Stats
}

func NewHttpProxyServer(cfg *conf.Config) (*HttpProxyServer, error) {
	pool := createNodePool(cfg)
	balancer, err := resolveLoadBalancer(cfg, pool)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	transport := createTransport(cfg)

	if cfg.HTTP.EnableHTTP2 {
		if err := http2.ConfigureTransport(transport); err != nil {
			cancel()
			return nil, err
		}
	}

	var rt *router.Router
	if cfg.HTTP != nil && len(cfg.HTTP.Routes) > 0 {
		rt = router.NewRouter(cfg.HTTP.Routes, pool)
	}

	securityManager := createSecurityManager(ctx, cfg)

	httpServer := &HttpProxyServer{
		config:          cfg,
		pool:            pool,
		balancer:        balancer,
		router:          rt,
		transport:       transport,
		securityManager: securityManager,
		ctx:             ctx,
		cancelFunc:      cancel,
		stats:           &Stats{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", httpServer.handleRequest)

	httpServer.server = &http.Server{
		Addr:           cfg.Listen,
		Handler:        mux,
		ReadTimeout:    cfg.Timeouts.Read,
		WriteTimeout:   cfg.Timeouts.Write,
		IdleTimeout:    cfg.Timeouts.Idle,
		MaxHeaderBytes: MaxHeaderBytes,
	}

	if cfg.HTTP.EnableHTTP2 {
		if err := http2.ConfigureServer(httpServer.server, &http2.Server{}); err != nil {
			cancel()
			return nil, err
		}
	}

	return httpServer, nil
}

func createTransport(cfg *conf.Config) *http.Transport {
	return &http.Transport{
		MaxIdleConnsPerHost: cfg.HTTP.MaxIdleConnsPerHost,
		IdleConnTimeout:     cfg.HTTP.IdleConnTimeout,
		DisableKeepAlives:   false,
		DisableCompression:  false,
		DialContext: (&net.Dialer{
			Timeout:   cfg.Timeouts.Connect,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     cfg.HTTP.EnableHTTP2,
		MaxIdleConns:          100,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: cfg.Timeouts.Read,
		WriteBufferSize:       4096,
		ReadBufferSize:        4096,
	}
}

func (h *HttpProxyServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	h.stats.OnRequestStart()
	defer h.stats.OnRequestEnd()

	if h.securityManager != nil {
		clientIP := getClientIP(r)
		allowed, err := h.securityManager.AllowConn(clientIP)
		if err != nil || !allowed {
			slog.Warn("request rejected by security manager",
				"client_ip", clientIP,
				"path", r.URL.Path,
				"error", err)
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
	}

	if h.config.HTTP.EnableWebSocket && isWebSocketRequest(r) {
		h.handleWebSocket(w, r)
		return
	}

	clientIP := getClientIP(r)
	selectedBackend := h.getNodeForRequest(w, r)
	if selectedBackend == nil {
		return
	}

	selectedBackend.IncrementActiveConnections()
	defer selectedBackend.DecrementActiveConnections()

	targetURL := &url.URL{
		Scheme: getScheme(r),
		Host:   selectedBackend.Address(),
	}

	proxy := h.createReverseProxy(targetURL, selectedBackend, clientIP)
	proxy.ServeHTTP(w, r)
}

func (h *HttpProxyServer) createReverseProxy(target *url.URL, node *node.Node, clientIP string) *httputil.ReverseProxy {
	scheme := target.Scheme
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.SetXForwarded()
			pr.Out.Header.Set("X-Real-IP", clientIP)
			pr.Out.Header.Set("X-Forwarded-Proto", scheme)
		},
		Transport: h.transport,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("Backend error for %s: %v", node.Address(), err)
			node.MarkUnhealthy()
			http.Error(w, "Backend error", http.StatusBadGateway)
		},
	}
	return proxy
}

func (h *HttpProxyServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if h.securityManager != nil {
		clientIP := getClientIP(r)
		allowed, err := h.securityManager.AllowConn(clientIP)
		if err != nil || !allowed {
			slog.Warn("websocket request rejected by security manager",
				"client_ip", clientIP,
				"path", r.URL.Path,
				"error", err)
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
	}

	node := h.getNodeForRequest(w, r)
	if node == nil {
		return
	}

	node.IncrementActiveConnections()
	defer node.DecrementActiveConnections()

	backendConn, err := net.DialTimeout("tcp", node.Address(), h.config.Timeouts.Connect)
	if err != nil {
		node.MarkUnhealthy()
		http.Error(w, "Failed to connect to backend", http.StatusBadGateway)
		return
	}
	defer backendConn.Close()

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "WebSocket hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, bufrw, err := hijacker.Hijack()
	if err != nil {
		log.Printf("Failed to hijack connection: %v", err)
		http.Error(w, "Failed to hijack connection", http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	if bufrw != nil {
		if err := bufrw.Flush(); err != nil {
			log.Printf("Failed to flush buffered data: %v", err)
		}
	}

	if err := r.Write(backendConn); err != nil {
		log.Printf("Failed to write upgrade request: %v", err)
		return
	}

	h.proxyWebSocket(clientConn, backendConn)
}

func (h *HttpProxyServer) getNodeForRequest(w http.ResponseWriter, r *http.Request) *node.Node {
	var selectedBackend *node.Node
	clientIP := getClientIP(r)

	// If router is configured, use it to get the appropriate pool for this request
	targetPool := h.pool
	if h.router != nil {
		targetPool = h.router.Match(r)
	}

	// Use a temporary balancer for the matched pool if it's different from the default
	var balancer balance.LoadBalancer
	if targetPool != h.pool {
		// Create a temporary balancer for the matched pool using the same algorithm
		var err error
		balancer, err = resolveLoadBalancer(h.config, targetPool)
		if err != nil {
			http.Error(w, "Failed to select backend", http.StatusInternalServerError)
			return nil
		}
	} else {
		balancer = h.balancer
	}

	// Select backend based on balancer type
	switch b := balancer.(type) {
	case interface{ SelectWithKey(string) *node.Node }:
		selectedBackend = b.SelectWithKey(clientIP)
	case interface{ SelectWithClientIP(string) *node.Node }:
		selectedBackend = b.SelectWithClientIP(clientIP)
	default:
		selectedBackend, _ = balancer.Select()
	}

	if selectedBackend == nil {
		http.Error(w, "No healthy backend available", http.StatusServiceUnavailable)
		return nil
	}
	return selectedBackend
}

func (h *HttpProxyServer) proxyWebSocket(clientConn, backendConn net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Backend
	go func() {
		defer wg.Done()
		_, err := io.Copy(backendConn, clientConn)
		if err != nil && err != io.EOF {
			log.Printf("Error copying WebSocket client -> backend: %v", err)
		}
		if tcpConn, ok := backendConn.(*net.TCPConn); ok {
			if err := tcpConn.CloseWrite(); err != nil {
				log.Printf("Error closing write side of backend connection: %v", err)
			}
		}
	}()

	// Backend -> Client
	go func() {
		defer wg.Done()
		_, err := io.Copy(clientConn, backendConn)
		if err != nil && err != io.EOF {
			log.Printf("Error copying WebSocket backend -> client: %v", err)
		}
		if tcpConn, ok := clientConn.(*net.TCPConn); ok {
			if err := tcpConn.CloseWrite(); err != nil {
				log.Printf("Error closing write side of client connection: %v", err)
			}
		}
	}()

	wg.Wait()
}

func (h *HttpProxyServer) Start() error {
	var g errgroup.Group
	g.Go(func() error {
		var err error

		switch {
		case h.config.TLS != nil && h.config.TLS.Enabled:
			err = h.startTLS()
		default:
			err = h.startRegularHTTP()
		}

		return err
	})
	return g.Wait()
}

func (h *HttpProxyServer) startTLS() error {
	terminator, err := createTerminator(h.config.TLS)
	if err != nil {
		return err
	}

	if err := terminator.Listen(h.config.Listen); err != nil {
		return err
	}

	h.terminator = terminator

	slog.Info("HTTPS proxy server started",
		"listen", h.config.Listen,
		"mode", h.config.Mode,
		"tls", "enabled",
		"http2", h.config.HTTP.EnableHTTP2)

	if err := h.server.Serve(terminator); err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}

func (h *HttpProxyServer) startRegularHTTP() error {
	slog.Info("HTTP proxy server started",
		"listen", h.config.Listen,
		"mode", h.config.Mode,
		"http2", h.config.HTTP.EnableHTTP2)

	if err := h.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (h *HttpProxyServer) Shutdown() error {
	slog.Info("shutting down the server")

	h.cancelFunc()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := h.server.Shutdown(ctx); err != nil {
		slog.Error("Error during HTTP server shutdown", "error", err)
	}

	if h.terminator != nil {
		if err := h.terminator.Close(); err != nil {
			slog.Error("Error closing TLS terminator", "error", err)
		}
	}

	h.transport.CloseIdleConnections()

	return nil
}

func isWebSocketRequest(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("Upgrade")) == "websocket" && strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}
