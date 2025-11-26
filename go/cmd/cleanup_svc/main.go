package main

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"time"
)

func main() {
	const socketPath = "/tmp/ipc.sock"

	ln, err := net.Dial("unix", socketPath)
	if err != nil {
		panic(err)
	}
	defer ln.Close()

	slog.Info("Connected to IPC socket at ", "addr", ln.RemoteAddr().String())

	scanner := bufio.NewScanner(ln)
	for {
		n, err := fmt.Fprintln(ln, "11")
		if err != nil || n == 0 {
			slog.Error("Failed to write to IPC socket", "error", err)
			return
		}

		if scanner.Scan() {
			slog.Info("Received from IPC socket", "message", scanner.Text())
		}

		time.Sleep(10 * time.Second)
	}
}
