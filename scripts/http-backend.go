package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/net/websocket"
)

var (
	port     = flag.Int("port", 9001, "Port to listen on")
	name     = flag.String("name", "Backend", "Backend name")
	delay    = flag.Int("delay", 0, "Artificial delay in milliseconds")
	enableWS = flag.Bool("websocket", false, "Enable WebSocket endpoint")
)

func main() {
	flag.Parse()

	// Create HTTP server
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})

	// Main handler
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Add artificial delay if configured
		if *delay > 0 {
			time.Sleep(time.Duration(*delay) * time.Millisecond)
		}

		// Log request
		log.Printf("Request: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

		// Set response headers
		w.Header().Set("X-Backend-Name", *name)
		w.Header().Set("X-Backend-Port", fmt.Sprintf("%d", *port))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		// Write response
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>%s</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            max-width: 800px;
            margin: 50px auto;
            padding: 20px;
            background-color: #f5f5f5;
        }
        .container {
            background-color: white;
            padding: 30px;
            border-radius: 10px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
        }
        h1 {
            color: #333;
            border-bottom: 3px solid #4CAF50;
            padding-bottom: 10px;
        }
        .info {
            margin: 20px 0;
            padding: 15px;
            background-color: #e8f5e9;
            border-left: 4px solid #4CAF50;
        }
        .request-info {
            margin: 20px 0;
            padding: 15px;
            background-color: #e3f2fd;
            border-left: 4px solid #2196F3;
        }
        table {
            width: 100%%;
            border-collapse: collapse;
            margin-top: 10px;
        }
        th, td {
            padding: 8px;
            text-align: left;
            border-bottom: 1px solid #ddd;
        }
        th {
            background-color: #f0f0f0;
            font-weight: bold;
        }
        code {
            background-color: #f5f5f5;
            padding: 2px 6px;
            border-radius: 3px;
            font-family: monospace;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>%s</h1>

        <div class="info">
            <strong>Backend Information:</strong><br>
            <table>
                <tr><th>Name</th><td>%s</td></tr>
                <tr><th>Port</th><td>%d</td></tr>
                <tr><th>Hostname</th><td>%s</td></tr>
                <tr><th>Time</th><td>%s</td></tr>
            </table>
        </div>

        <div class="request-info">
            <strong>Request Information:</strong><br>
            <table>
                <tr><th>Method</th><td><code>%s</code></td></tr>
                <tr><th>Path</th><td><code>%s</code></td></tr>
                <tr><th>Client IP</th><td>%s</td></tr>
                <tr><th>User-Agent</th><td>%s</td></tr>
                <tr><th>Host</th><td>%s</td></tr>
            </table>
        </div>

        <div class="request-info">
            <strong>Request Headers:</strong><br>
            <table>
                <tr><th>Header</th><th>Value</th></tr>
`, *name, *name, *name, *port, getHostname(), time.Now().Format(time.RFC3339),
			r.Method, r.URL.Path, r.RemoteAddr, r.UserAgent(), r.Host)

		// Display all headers
		for name, values := range r.Header {
			for _, value := range values {
				fmt.Fprintf(w, "                <tr><td><code>%s</code></td><td>%s</td></tr>\n", name, value)
			}
		}

		fmt.Fprintf(w, `
            </table>
        </div>
    </div>
</body>
</html>
`)
	})

	if *enableWS {
		mux.Handle("/ws", websocket.Handler(handleWebSocket))
		log.Printf("WebSocket endpoint enabled at /ws")
	}

	// Start server
	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Starting HTTP backend server: %s on %s", *name, addr)
	log.Printf("Health check: http://localhost%s/health", addr)
	if *enableWS {
		log.Printf("WebSocket: ws://localhost%s/ws", addr)
	}

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func handleWebSocket(ws *websocket.Conn) {
	log.Printf("WebSocket connection from %s", ws.RemoteAddr())

	// Send welcome message
	welcomeMsg := fmt.Sprintf("Welcome to %s WebSocket server!", *name)
	if err := websocket.Message.Send(ws, welcomeMsg); err != nil {
		log.Printf("WebSocket send error: %v", err)
		return
	}

	// Echo loop
	for {
		var msg string
		if err := websocket.Message.Receive(ws, &msg); err != nil {
			log.Printf("WebSocket connection closed: %v", err)
			return
		}

		log.Printf("WebSocket message received: %s", msg)

		// Echo back with backend name
		response := fmt.Sprintf("[%s] Echo: %s", *name, msg)
		if err := websocket.Message.Send(ws, response); err != nil {
			log.Printf("WebSocket send error: %v", err)
			return
		}
	}
}

// getHostname returns the system hostname
func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}
