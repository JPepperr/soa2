package main

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/heetch/confita"
	"github.com/heetch/confita/backend/env"
	"github.com/heetch/confita/backend/flags"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	mafia_connection "mafia/protos"
	server "mafia/server/lib"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	cfg := server.Config{
		Port:          5050,
		StatsEndpoint: "http://[::]:6669/push",
		LogLevel:      "info",
	}

	err := confita.NewLoader(
		env.NewBackend(),
		flags.NewBackend(),
	).Load(context.Background(), &cfg)
	if err != nil {
		fmt.Println("Failed to read config", err)
		return
	}

	srv, err := server.InitServer(&cfg)
	if err != nil {
		fmt.Println("Failed to init server", err)
		return
	}

	addr := fmt.Sprintf(":%d", cfg.Port)
	srv.Logger.Info("Start to listen", zap.String("addr", addr))
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		srv.Logger.Fatal("Failed to listen", zap.Error(err))
	}
	grpcServer := grpc.NewServer()
	mafia_connection.RegisterMafiaServiceServer(grpcServer, srv)
	grpcServer.Serve(lis)
}
