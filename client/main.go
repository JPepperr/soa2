package main

import (
	"context"
	"fmt"
	client "mafia/client/lib"
	"math/rand"
	"time"

	"github.com/heetch/confita"
	"github.com/heetch/confita/backend/env"
	"github.com/heetch/confita/backend/flags"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	cfg := client.Config{
		BotMode:       true,
		ServerAddr:    "localhost:5050",
		RabbitmqCreds: "amqp://guest:guest@localhost:5672/",
	}
	err := confita.NewLoader(
		env.NewBackend(),
		flags.NewBackend(),
	).Load(context.Background(), &cfg)
	if err != nil {
		fmt.Println("Failed to read config", err)
		return
	}

	for {
		client := client.GetClient()
		client.Run(cfg.ServerAddr, cfg.RabbitmqCreds)
	}
}
