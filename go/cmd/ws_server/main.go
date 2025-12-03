package main

import (
	"log/slog"

	"github.com/ArditZubaku/go-node-ws/internal/connmanager"
	"github.com/ArditZubaku/go-node-ws/internal/http"
	"github.com/ArditZubaku/go-node-ws/internal/tcp"
)

func main() {
	slog.SetLogLoggerLevel(slog.LevelInfo)
	cm := connmanager.NewConnectionManager()
	go tcp.HandleCleanUpTask(cm)
	http.NewServer(cm).Start()
}
