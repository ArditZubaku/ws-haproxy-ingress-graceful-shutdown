// Package connmanager provides WebSocket connection management functionality.
// It tracks active connections and handles graceful shutdown procedures.
package connmanager

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ConnectionManager tracks and manages WebSocket connections
type ConnectionManager struct {
	connections []*websocket.Conn
	mu          sync.RWMutex
	Shutdown    chan struct{}
}

func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		connections: make([]*websocket.Conn, 100),
		Shutdown:    make(chan struct{}),
	}
}

func (cm *ConnectionManager) AddConnection(conn *websocket.Conn) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.connections = append(cm.connections, conn)
	slog.Info("WebSocket connection added", "total", len(cm.connections))
	if len(cm.connections) >= 100 {
		slog.Info("Reached 100 WebSocket connections")
	}
}

func (cm *ConnectionManager) RemoveConnection(index int) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.connections = slices.Delete(cm.connections, index, index+1)
	slog.Info("WebSocket connection removed", "total", len(cm.connections))
}

func (cm *ConnectionManager) GetFirstNConnections(n int) []*websocket.Conn {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	connections := make([]*websocket.Conn, n)
	copy(connections, cm.connections[:min(n, len(cm.connections))])

	return connections
}

func (cm *ConnectionManager) GetConnectionsCount() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return len(cm.connections)
}

func (cm *ConnectionManager) CloseFirstNConnections(n int) {
	connections := cm.GetFirstNConnections(n)

	slog.Info("Closing WebSocket connections", "count", len(connections))

	for i, conn := range connections {
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

		// Remove from array
		cm.RemoveConnection(i)
	}
}

func (cm *ConnectionManager) CloseAllConnections(ctx context.Context) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	slog.Info("Closing all WebSocket connections", "count", len(cm.connections))

	// Signal shutdown to all connections
	close(cm.Shutdown)

	// Close all connections gracefully
	for i, conn := range cm.connections {
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

		// Remove from array
		cm.RemoveConnection(i)
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
