// Package tcp provides inter-service communication functionality via TCP.
package tcp

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"strconv"

	"github.com/ArditZubaku/go-node-ws/internal/connmanager"
)

func HandleCleanUpTask(cm *connmanager.ConnectionManager) {
	ln, err := net.Listen("tcp", ":9999")
	if err != nil {
		slog.Error("Failed to listen on TCP port", "error", err)
		return
	}
	defer ln.Close()

	slog.Info("Service communication server listening on", "addr", ln.Addr().String())

	for {
		conn, err := ln.Accept()
		if err != nil {
			slog.Error("Failed to accept TCP connection", "error", err)
			continue
		}
		go handleServiceConnection(conn, cm)
	}
}

func handleServiceConnection(conn net.Conn, cm *connmanager.ConnectionManager) {
	defer conn.Close()

	reader := bufio.NewScanner(conn)

	for reader.Scan() {
		msg := reader.Text()
		n, err := strconv.Atoi(msg)
		if err != nil {
			slog.Error("Invalid number received", "error", err)
			continue
		}
		slog.Info("Received service message", "message", n)

		cm.CloseNConnections(n)

		// No need for newline, fmt.Fprintln adds it
		n, err = fmt.Fprintln(conn, "Closing "+msg+" WS connections")
		if n == 0 || err != nil {
			slog.Error("Failed to write service response", "error", err)
			return
		}
	}
}
