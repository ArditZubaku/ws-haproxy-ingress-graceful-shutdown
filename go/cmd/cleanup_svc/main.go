package main

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"time"
)

func main() {
	const wsServer = "ws-app:9999"

	preStopConn, preStopErr := net.Listen("tcp", ":55000")
	if preStopErr != nil {
		panic(preStopErr)
	}
	defer preStopConn.Close()

	slog.Info("Pre-stop TCP listener started on", "addr", preStopConn.Addr().String())

	// Once we receive a connection, we know preStop reached us and we can start
	preStopConn.Accept()
	slog.Info("Pre-stop hook triggered, connecting to service...")

	conn, err := net.Dial("tcp", wsServer)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	slog.Info("Connected to service at ", "addr", conn.RemoteAddr().String())

	scanner := bufio.NewScanner(conn)
	for {
		n, err := fmt.Fprintln(conn, "11")
		if err != nil || n == 0 {
			slog.Error("Failed to write to service", "error", err)
			return
		}

		if scanner.Scan() {
			slog.Info("Received from service", "message", scanner.Text())
		}

		time.Sleep(10 * time.Second)
	}
}
