package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/utkarsh5026/balance/pkg/conf"
	"github.com/utkarsh5026/balance/pkg/proxy"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Parse()

	cfg, err := conf.Load(*configPath, true)
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	var server *proxy.ProxyServer
	slog.Info("Configuration loaded successfully from " + *configPath)
	switch cfg.Mode {
	case "tcp":
		server, err = proxy.NewTCPServer(cfg)
	default:
		slog.Error("Unsupported mode: " + cfg.Mode)
		os.Exit(1)
	}

	if err != nil {
		slog.Error("Failed to create proxy server", "error", err)
		os.Exit(1)
	}

	if err := server.Start(); err != nil {
		slog.Error("Failed to start proxy server", "error", err)
		os.Exit(1)
	}

	slog.Info("Proxy server started in " + cfg.Mode + " mode, listening on " + cfg.Listen)
	waitForShutdown(server)
}

// waitForShutdown waits for interrupt signal and gracefully shuts down the server
func waitForShutdown(server *proxy.ProxyServer) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	slog.Info("Shutdown signal received, gracefully shutting down...")

	if err := server.Shutdown(); err != nil {
		slog.Error("Error during shutdown", "error", err)
	}

	slog.Info("Server stopped")
}
