package server

import (
	"fmt"
	"io"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	game "mafia/game"
	mafia_connection "mafia/protos"
)

type Config struct {
	Port     uint32 `config:"port"`
	LogLevel string `config:"log-level"`
}

type Server struct {
	playersToRooms map[uint64]uint64
	rooms          map[uint64]*game.Room
	Logger         *zap.Logger
	mux            sync.Mutex

	mafia_connection.UnimplementedMafiaServiceServer
}

var Logger *zap.Logger = zap.NewNop()

func InitLogger(logLevel string) (*zap.Logger, error) {
	var level zapcore.Level
	switch logLevel {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warning":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	default:
		return nil, fmt.Errorf("unknown log level: %s", logLevel)
	}
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(level)
	var err error
	Logger, err = config.Build()
	if err != nil {
		return nil, err
	}
	return Logger, nil
}

func InitServer(cfg *Config) (*Server, error) {
	logger, err := InitLogger(cfg.LogLevel)
	if err != nil {
		fmt.Println("Failed to create logger", err)
		return nil, err
	}

	return &Server{
		playersToRooms: make(map[uint64]uint64),
		rooms:          make(map[uint64]*game.Room),
		mux:            sync.Mutex{},
		Logger:         logger,
	}, nil
}

func (s *Server) AddPlayer(user *mafia_connection.User, stream mafia_connection.MafiaService_RouteGameServer) uint64 {
	s.mux.Lock()
	defer s.mux.Unlock()
	id, ok := s.playersToRooms[user.ID]
	if ok {
		return id
	}
	for id := range s.rooms {
		if s.rooms[id].TryToAddPlayer(user, stream) {
			s.playersToRooms[user.ID] = id
			return id
		}
	}
	room := game.GetNewRoom()
	s.rooms[room.ID] = room
	room.TryToAddPlayer(user, stream)
	s.playersToRooms[user.ID] = room.ID
	return room.ID
}

func (s *Server) HandlePlayersActions(
	stopJobs chan bool,
	errChan chan error,
	jobs *sync.WaitGroup,
	stream mafia_connection.MafiaService_RouteGameServer,
) {
	var curUserData *mafia_connection.User
	defer jobs.Done()
	for {
		select {
		case <-stopJobs:
			return
		default:
			playerAction, err := stream.Recv()
			if err != nil {
				if curUserData != nil {
					id, ok := s.playersToRooms[curUserData.ID]
					if ok {
						s.rooms[id].LeaveRoom(curUserData)
					}
				}
				errChan <- err
				return
			}

			switch {
			case playerAction.GetConnetion() != nil:
				user := playerAction.GetConnetion()
				curUserData = user
				id := s.AddPlayer(user, stream)
				s.rooms[id].JoinRoom(user)
			case playerAction.GetVote() != nil:
				roomId := s.playersToRooms[curUserData.ID]
				s.rooms[roomId].VoteRequest(curUserData, playerAction.GetVote())
			case playerAction.GetShow() != nil:
				roomId := s.playersToRooms[curUserData.ID]
				s.rooms[roomId].ShowRequest(curUserData, playerAction.GetShow())
			}
		}
	}
}

func (s *Server) RouteGame(stream mafia_connection.MafiaService_RouteGameServer) error {
	stopJobs := make(chan bool)
	errChan := make(chan error)
	var jobs sync.WaitGroup
	jobs.Add(1)
	defer jobs.Wait()

	go s.HandlePlayersActions(stopJobs, errChan, &jobs, stream)

	err := <-errChan
	close(errChan)
	close(stopJobs)
	if err == io.EOF {
		return nil
	}
	s.Logger.Error("route", zap.Error(err))
	return err
}
