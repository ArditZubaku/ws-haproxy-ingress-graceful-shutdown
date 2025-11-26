// Package handlers provides HTTP and WebSocket request handlers for the server.
package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/ArditZubaku/go-node-ws/internal/conn_manager"
	"github.com/gorilla/websocket"
)

func RootHandler(cm *conn_manager.ConnectionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
	}
}

func HealthzHandler(w http.ResponseWriter, r *http.Request) {
	// Only log health checks at debug level to reduce noise
	slog.Debug(
		"HealthzHandler received request:",
		"method", r.Method,
		"path", r.URL.Path,
		"remote_addr", r.RemoteAddr,
		"user_agent", r.UserAgent(),
	)
	switch r.Method {
	case http.MethodGet, http.MethodOptions:
		// Allow HAProxy OPTIONS / K8s GET
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]any{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.Error("Failed to encode health response", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for testing
	},
}

func wsHandler(w http.ResponseWriter, r *http.Request, cm *conn_manager.ConnectionManager) {
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
		case <-cm.Shutdown:
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
					case <-cm.Shutdown:
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
