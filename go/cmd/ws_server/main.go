package main

import (
	"log/slog"

	"github.com/ArditZubaku/go-node-ws/internal/conn_manager"
	"github.com/ArditZubaku/go-node-ws/internal/ipc"
	"github.com/ArditZubaku/go-node-ws/internal/server"
)

func main() {
	slog.SetLogLoggerLevel(slog.LevelInfo)
	go ipc.HandleIPCCommunication()
	server.NewServer(conn_manager.NewConnectionManager()).Start()
}
