package proxy

type Server interface {
	// Start starts the server.
	Start() error
	// Shutdown gracefully shuts down the server.
	Shutdown() error
}
