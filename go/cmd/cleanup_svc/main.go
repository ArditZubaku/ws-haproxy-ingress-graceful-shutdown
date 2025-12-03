package main

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"
)

func main() {
	const wsServer = "ws-app:9999"
	continueCh := make(chan struct{})
	go listenForPreStop(continueCh)

	<-continueCh
	slog.Info("Pre-stop signal received, starting cleanup...")
	performCleanupTask(wsServer)
}

func performCleanupTask(wsServer string) {
	conn, err := net.Dial("tcp", wsServer)
	if err != nil {
		panic(err)
	}
	defer closeOrLog(conn, "TCP connection")

	slog.Info("Connected to WS Server at ", "addr", conn.RemoteAddr().String())

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

func listenForPreStop(continueCh chan<- struct{}) {
	ln, err := net.Listen("tcp", ":55000")
	if err != nil {
		panic(err)
	}
	defer closeOrLog(ln, "listener")

	slog.Info("Pre-stop TCP listener started on", "addr", ln.Addr().String())

	sBuf := make([]byte, 1024)

	for {
		conn, err := ln.Accept()
		if err != nil {
			slog.Error("Failed to accept connection", "error", err)
			continue
		}
		slog.Info(
			"Accepted a new connection from",
			slog.String("addr", conn.RemoteAddr().String()),
		)

		go handleConnection(conn, sBuf, continueCh)
	}
}

func handleConnection(conn net.Conn, sBuf []byte, continueCh chan<- struct{}) {
	defer closeOrLog(conn, "TCP connection")

	// Listen for messages constantly until we receive "preStop-trigger"
	for {
		n, err := conn.Read(sBuf)
		slog.Info("Read from connection", "bytes", n)
		if err != nil {
			if err != io.EOF {
				slog.Error("Failed to read from connection", "error", err)
			}
			return
		}

		if n == 0 {
			return
		}

		message := string(sBuf[:n])
		slog.Info("Received message", "message", message, "from", conn.RemoteAddr())

		if message == "preStop-trigger\n" { // Because echo adds newline
			slog.Info("Stop message received, triggering shutdown")
			close(continueCh)
			return
		}
	}
}

func closeOrLog(c io.Closer, part string) {
	if err := c.Close(); err != nil {
		slog.Error("Failed to close ->", "part", part)
	}
}
