package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	cm := NewConnectionManager()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Check if this is a WebSocket upgrade request
		if websocket.IsWebSocketUpgrade(r) {
			wsHandler(w, r, cm)
			return
		}

		// Regular HTTP request - return a simple response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := fmt.Sprintf(
			`{"message": "HTTP server is running", "timestamp": "%s", "method": "%s", "path": "%s"}`,
			time.Now().Format(time.RFC3339),
			r.Method,
			r.URL.Path,
		)
		w.Write([]byte(response))
		slog.Info("HTTP request handled", "method", r.Method, "path", r.URL.Path)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := map[string]any{
			"status":    "healthy",
			"timestamp": time.Now().Unix(),
		}

		err := json.NewEncoder(w).Encode(response)
		if err != nil {
			slog.Error("Failed to encode health response", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	})

	server := new(http.Server)
	server.Addr = ":8080"
	server.Handler = mux

	// Register shutdown callback for WebSocket connections
	server.RegisterOnShutdown(func() {
		slog.Info("Server shutdown initiated, closing WebSocket connections...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cm.CloseAllConnections(ctx)
	})

	ln, err := net.Listen("tcp", server.Addr)
	if err != nil {
		slog.Error("Error starting server", "error", err)
	}
	defer ln.Close()

	go func() {
		slog.Info("Starting server", "address", server.Addr)
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("Server error", "error", err)
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	<-shutdown
	slog.Info("Received shutdown signal", "signal", <-shutdown)

	slog.Info("Shutting down the server gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
		if err := server.Close(); err != nil {
			slog.Error("Error closing server", "error", err)
			panic(err)
		}
	}
	slog.Info("Server exited properly")
}

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for testing
	},
}

// ConnectionManager tracks and manages WebSocket connections
type ConnectionManager struct {
	connections map[*websocket.Conn]bool
	mu          sync.RWMutex
	shutdown    chan struct{}
}

func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		connections: make(map[*websocket.Conn]bool),
		shutdown:    make(chan struct{}),
	}
}

func (cm *ConnectionManager) AddConnection(conn *websocket.Conn) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.connections[conn] = true
	slog.Info("WebSocket connection added", "total", len(cm.connections))
}

func (cm *ConnectionManager) RemoveConnection(conn *websocket.Conn) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.connections, conn)
	slog.Info("WebSocket connection removed", "total", len(cm.connections))
}

func (cm *ConnectionManager) CloseAllConnections(ctx context.Context) {
	cm.mu.RLock()
	connections := make([]*websocket.Conn, 0, len(cm.connections))
	for conn := range cm.connections {
		connections = append(connections, conn)
	}
	cm.mu.RUnlock()

	slog.Info("Closing all WebSocket connections", "count", len(connections))

	// Signal shutdown to all connections
	close(cm.shutdown)

	// Close all connections gracefully
	for _, conn := range connections {
		// Send close message
		if err := conn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(
				websocket.CloseGoingAway,
				"Server shutting down",
			),
		); err != nil {
			slog.Error("Error sending close message", "error", err)
		}

		if err := conn.Close(); err != nil {
			slog.Error("Error closing WebSocket connection", "error", err)
		}
	}

	// Wait for all connections to be removed or timeout
	timeout := time.NewTimer(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer timeout.Stop()
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Warn("Context cancelled while waiting for WebSocket connections to close")
			return
		case <-timeout.C:
			slog.Warn("Timeout waiting for WebSocket connections to close")
			return
		case <-ticker.C:
			cm.mu.RLock()
			count := len(cm.connections)
			cm.mu.RUnlock()
			if count == 0 {
				slog.Info("All WebSocket connections closed")
				return
			}
		}
	}
}

func wsHandler(w http.ResponseWriter, r *http.Request, cm *ConnectionManager) {
	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("Failed to upgrade to WebSocket", "error", err)
		return
	}

	// Add connection to manager
	cm.AddConnection(conn)

	// Ensure connection is cleaned up
	defer func() {
		cm.RemoveConnection(conn)
		conn.Close()
	}()

	// Send welcome message
	if err := conn.WriteMessage(websocket.TextMessage, []byte("WebSocket connection established")); err != nil {
		slog.Error("Failed to send welcome message", "error", err)
		return
	}

	// Simple message handling loop - NO TIMEOUTS, NO PANIC RECOVERY
	for {
		select {
		case <-cm.shutdown:
			slog.Info("WebSocket connection shutting down due to server shutdown")
			return
		default:
			// Just read messages - let it block until a message comes or connection closes
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				// Connection closed or error occurred
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					slog.Info("WebSocket connection closed normally")
				} else {
					slog.Info("WebSocket connection error", "error", err)
				}
				return
			}

			slog.Info("Received message", "message", string(message))

			// Check if this is a slow request
			if string(message) == "SLOW_REQUEST" || string(message)[:9] == "SLOW_PING" {
				slog.Info("Processing slow request via WebSocket...")

				// Simulate slow work with shutdown awareness
				ticker := time.NewTicker(1 * time.Second)
				defer ticker.Stop()

				startTime := time.Now()
				for elapsed := time.Duration(0); elapsed < 30*time.Second; elapsed = time.Since(startTime) {
					select {
					case <-cm.shutdown:
						slog.Info("Slow WebSocket request interrupted by shutdown", "elapsed", elapsed)
						response := fmt.Sprintf("SLOW_INTERRUPTED: Request interrupted by server shutdown after %.1f seconds", elapsed.Seconds())
						if err := conn.WriteMessage(messageType, []byte(response)); err != nil {
							slog.Error("Failed to write interruption message", "error", err)
						}
						return
					case <-ticker.C:
						// Continue waiting
					}
				}

				response := fmt.Sprintf("SLOW_COMPLETE: Slow operation completed after 30 seconds at %s", time.Now().Format(time.RFC3339))
				if err := conn.WriteMessage(messageType, []byte(response)); err != nil {
					slog.Error("Failed to write slow response", "error", err)
					return
				}
				slog.Info("Slow WebSocket operation completed")
			} else {
				// Regular echo response
				response := "Echo: " + string(message)
				if err := conn.WriteMessage(messageType, []byte(response)); err != nil {
					slog.Error("Failed to write echo", "error", err)
					return
				}
				slog.Info("Sent echo back to client")
			}
		}
	}
}
