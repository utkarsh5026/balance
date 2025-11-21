package proxy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/utkarsh5026/balance/pkg/balance"
	"github.com/utkarsh5026/balance/pkg/conf"
	"github.com/utkarsh5026/balance/pkg/node"
	"github.com/utkarsh5026/balance/pkg/transport"
)

type ProxyServer struct {
	listener   net.Listener
	terminator *transport.Terminator
	config     *conf.Config
	pool       *node.Pool
	balancer   balance.LoadBalancer

	ctx    context.Context
	cancel context.CancelFunc
	conns  sync.Map

	totalBytesRecieved atomic.Uint64
	totalBytesSent     atomic.Uint64
	totalConnections   atomic.Uint64
	activeConnections  atomic.Int64
}

func NewTCPServer(cfg *conf.Config) (*ProxyServer, error) {
	pool := createNodePool(cfg)

	balancer, err := resolveLoadBalancer(cfg, pool)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &ProxyServer{
		config:   cfg,
		pool:     pool,
		balancer: balancer,
		ctx:      ctx,
		cancel:   cancel,
	}, nil
}

func (s *ProxyServer) Start() error {
	var err error

	switch {
	case s.config.TLS != nil && s.config.TLS.Enabled:
		err = s.startTLS()
	default:
		err = s.startRegularTCP()
	}

	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Go(func() {
		s.startAccepting()
	})

	wg.Wait()
	return nil
}

func (s *ProxyServer) startTLS() error {
	terminator, err := createTerminator(s.config.TLS)
	if err != nil {
		return fmt.Errorf("failed to create TLS terminator: %w", err)
	}

	if err := terminator.Listen(s.config.Listen); err != nil {
		return fmt.Errorf("failed to start TLS listener: %w", err)
	}

	s.terminator = terminator
	s.listener = terminator

	slog.Info("TLS proxy server started",
		"listen", s.config.Listen,
		"mode", s.config.Mode,
		"load_balancer", s.config.LoadBalancer.Algorithm,
		"tls", "enabled")

	return nil
}

func (s *ProxyServer) startRegularTCP() error {
	ln, err := net.Listen("tcp", s.config.Listen)
	if err != nil {
		return fmt.Errorf("failed to start listener: %w", err)
	}
	s.listener = ln

	slog.Info("proxy server started",
		"listen", s.config.Listen,
		"mode", s.config.Mode,
		"load_balancer", s.config.LoadBalancer.Algorithm)
	return nil
}

func (s *ProxyServer) startAccepting() {
	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return

			default:
				slog.Error("failed to accept connection", "error", err)
				continue
			}
		}

		wg.Go(func() {
			s.handleConnection(conn)
		})
	}
}

func (s *ProxyServer) handleConnection(clientConn net.Conn) {
	s.conns.Store(clientConn, struct{}{})
	defer s.conns.Delete(clientConn)
	defer clientConn.Close()

	s.totalConnections.Add(1)
	s.activeConnections.Add(1)
	defer s.activeConnections.Add(-1)

	node, err := s.balancer.Select()
	if err != nil {
		slog.Error("failed to select backend node", "error", err)
		return
	}

	dialer := net.Dialer{
		Timeout: s.config.Timeouts.Connect,
	}

	backendConn, err := dialer.DialContext(s.ctx, "tcp", node.Address())
	if err != nil {
		slog.Error("failed to connect to backend node", "node", node.Address(), "error", err)
		return
	}

	defer backendConn.Close()

	if s.config.Timeouts.Read > 0 {
		deadLine := time.Now().Add(s.config.Timeouts.Read)
		clientConn.SetReadDeadline(deadLine)
		backendConn.SetReadDeadline(deadLine)
	}

	if s.config.Timeouts.Write > 0 {
		deadLine := time.Now().Add(s.config.Timeouts.Write)
		clientConn.SetWriteDeadline(deadLine)
		backendConn.SetWriteDeadline(deadLine)
	}

	s.proxyData(clientConn, backendConn)
}

func (s *ProxyServer) proxyData(client, backend net.Conn) {
	var wg sync.WaitGroup

	wg.Go(func() {
		n, err := io.Copy(backend, client)
		if err != nil {
			slog.Error("error while proxying data", "error", err)
		}

		s.totalBytesSent.Add(uint64(n))
		if tcpConn, ok := backend.(*net.TCPConn); ok {
			tcpConn.CloseWrite()
		}
	})

	wg.Go(func() {
		n, err := io.Copy(client, backend)
		if err != nil {
			slog.Error("error while proxying data", "error", err)
		}

		s.totalBytesRecieved.Add(uint64(n))
		if tcpConn, ok := client.(*net.TCPConn); ok {
			tcpConn.CloseWrite()
		}
	})

	wg.Wait()
}

func (s *ProxyServer) Shutdown() error {
	slog.Info("shutting down proxy server")
	s.cancel()

	if s.terminator != nil {
		if err := s.terminator.Close(); err != nil {
			slog.Error("Error closing TLS terminator", "error", err)
		}
	} else if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			slog.Error("Error closing listener", "error", err)
		}
	}

	s.conns.Range(func(key, value any) bool {
		if conn, ok := key.(net.Conn); ok {
			conn.Close()
		}
		return true
	})

	slog.Info("Final statistics:")
	slog.Info("  Total connections", "count", s.totalConnections.Load())
	slog.Info("  Bytes received", "count", s.totalBytesRecieved.Load())
	slog.Info("  Bytes sent", "count", s.totalBytesSent.Load())
	return nil
}
