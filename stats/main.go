package main

import (
	"context"
	"fmt"
	"mafia/stats/lib/server"
	"math/rand"
	"time"

	// _ "github.com/99designs/gqlgen"
	"github.com/heetch/confita"
	"github.com/heetch/confita/backend/env"
	"github.com/heetch/confita/backend/flags"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	cfg := server.Config{
		UserStatsPort: 6669,
		GameStatsPort: 7776,
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
