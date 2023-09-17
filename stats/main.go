package main

import (
	"context"
	"fmt"
	"mafia/stats/lib/server"

	"github.com/heetch/confita"
	"github.com/heetch/confita/backend/env"
	"github.com/heetch/confita/backend/flags"
)

func main() {
	cfg := server.Config{
		Port: 6669,
	}

	err := confita.NewLoader(
		env.NewBackend(),
		flags.NewBackend(),
	).Load(context.Background(), &cfg)
	if err != nil {
		fmt.Println("Failed to read config", err)
		return
	}
	server := server.InitServer()
	server.Run(&cfg)
}
