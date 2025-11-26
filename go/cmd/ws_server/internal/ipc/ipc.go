// Package ipc provides inter-process communication functionality via Unix sockets.
package ipc

import (
	"bufio"
	"log/slog"
	"net"
	"os"
)

func HandleIPCCommunication() {
	const socketPath = "/tmp/ipc.sock"
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		panic(err)
	}

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		slog.Error("Failed to listen on unix socket", "error", err)
		return
	}
	defer ln.Close()

	slog.Info("IPC server listening on", "addr", ln.Addr().String())

	for {
		conn, err := ln.Accept()
		if err != nil {
			slog.Error("Failed to accept IPC connection", "error", err)
			continue
		}
		go handleIPCConnection(conn)
	}
}

func handleIPCConnection(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewScanner(conn)

	for reader.Scan() {
		msg := reader.Text()
		slog.Info("Received IPC message", "message", msg)

		resp := "Closing " + msg + " WS connections\n"

		// TODO: Integrate with connection manager to close connections based on msg

		n, err := conn.Write([]byte(resp))
		if n == 0 || err != nil {
			slog.Error("Failed to write IPC response", "error", err)
			return
		}
	}
}
