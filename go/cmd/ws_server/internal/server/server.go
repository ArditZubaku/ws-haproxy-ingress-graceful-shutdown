// Package server provides HTTP server functionality with WebSocket support and graceful shutdown.
package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ArditZubaku/go-node-ws/internal/conn_manager"
	"github.com/ArditZubaku/go-node-ws/internal/handlers"
)

type Server struct {
	cm   *conn_manager.ConnectionManager
	http *http.Server
	mux  *http.ServeMux
}

func NewServer(cm *conn_manager.ConnectionManager) *Server {
	mux := http.NewServeMux()

	s := &Server{
		cm:  cm,
		mux: mux,
		http: &http.Server{
			Addr:         ":8080",
			Handler:      mux,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
	}

	// Routes
	mux.HandleFunc("/", handlers.RootHandler(cm))
	mux.HandleFunc("/healthz", handlers.HealthzHandler)

	// WebSocket cleanup when server shuts down
	s.http.RegisterOnShutdown(func() {
		slog.Info("Closing all WebSocket connections...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.cm.CloseAllConnections(ctx)
	})

	return s
}

func (s *Server) Start() {
	ln, err := net.Listen("tcp", s.http.Addr)
	if err != nil {
		slog.Error("Failed to bind listener", "error", err)
		os.Exit(1)
	}

	go s.handleShutdown()

	slog.Info("HTTP Server starting", "addr", s.http.Addr)

	if err := s.http.Serve(ln); err != nil && err != http.ErrServerClosed {
		slog.Error("Server error", "error", err)
	}
}

func (s *Server) handleShutdown() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	<-sig
	slog.Info("Shutdown signal received, shutting down HTTP server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.http.Shutdown(ctx); err != nil {
		slog.Error("Forced shutdown", "error", err)
	}
}
