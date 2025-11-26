package conn_manager

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ConnectionManager tracks and manages WebSocket connections
type ConnectionManager struct {
	connections map[*websocket.Conn]bool
	mu          sync.RWMutex
	Shutdown    chan struct{}
}

func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		connections: make(map[*websocket.Conn]bool),
		Shutdown:    make(chan struct{}),
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
	close(cm.Shutdown)

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
