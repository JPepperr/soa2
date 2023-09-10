package game

import (
	"fmt"
	mafia_connection "mafia/protos"
	"math/rand"
	"strconv"
	"sync"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func SimpleMessage(num int) *mafia_connection.ServerAction {
	return &mafia_connection.ServerAction{
		Action: &mafia_connection.ServerAction_ChatMessage{
			ChatMessage: &mafia_connection.ChatMessage{
				Text: strconv.Itoa(num),
				Author: &mafia_connection.User{
					ID:       0,
					Nickname: "server",
				},
			},
		},
	}
}

type Player struct {
	connection mafia_connection.MafiaService_RouteGameServer
	info       mafia_connection.Player
}

type Room struct {
	ID      uint64
	state   mafia_connection.State
	players []*Player

	mux sync.Mutex
}

func (r *Room) TryToAddPlayer(user *mafia_connection.User, stream mafia_connection.MafiaService_RouteGameServer) bool {
	r.mux.Lock()
	defer r.mux.Unlock()
	if len(r.players) < 4 && r.state == mafia_connection.State_NOT_STARTED {
		r.players = append(r.players, &Player{
			connection: stream,
			info: mafia_connection.Player{
				User:  user,
				Role:  mafia_connection.Role_UNKNOWN,
				Alive: true,
			},
		})
		return true
	}
	return false
}

func (r *Room) getRoomInfoForPlayer(id uint64) *mafia_connection.RoomInfo {
	roomInfo := &mafia_connection.RoomInfo{}
	roomInfo.RoomID = r.ID
	roomInfo.State = r.state
	players := make([]*mafia_connection.Player, len(r.players))
	if r.state == mafia_connection.State_NOT_STARTED || r.state == mafia_connection.State_END {
		for i := range r.players {
			players[i] = &r.players[i].info
		}
	} else {
		role := mafia_connection.Role_UNKNOWN
		for i := range r.players {
			if r.players[i].info.User.ID == id {
				role = r.players[i].info.Role
			}
		}
		for i := range r.players {
			rightRole := mafia_connection.Role_UNKNOWN
			if r.players[i].info.User.ID == id || (r.players[i].info.Role == role && role != mafia_connection.Role_CIVILIAN) {
				rightRole = role
			}
			players[i] = &mafia_connection.Player{
				User:  r.players[i].info.User,
				Role:  rightRole,
				Alive: r.players[i].info.Alive,
			}
		}
	}
	roomInfo.Players = players
	return roomInfo
}

func (r *Room) sendForAll(message string) {
	for id, player := range r.players {
		player.connection.Send(&mafia_connection.ServerAction{
			Action: &mafia_connection.ServerAction_Event{
				Event: &mafia_connection.RoomEvent{
					Event:    wrapperspb.String(message),
					RoomInfo: r.getRoomInfoForPlayer(r.players[id].info.User.ID),
				},
			},
		})
	}
}

func (r *Room) startGame() {
	r.state = mafia_connection.State_NIGHT
	roles := []mafia_connection.Role{
		mafia_connection.Role_CIVILIAN,
		mafia_connection.Role_CIVILIAN,
		mafia_connection.Role_MAFIA,
		mafia_connection.Role_SHERIFF,
	}
	rand.Shuffle(len(roles), func(i, j int) { roles[i], roles[j] = roles[j], roles[i] })
	for i := range r.players {
		r.players[i].info.Role = roles[i]
	}
}

func (r *Room) JoinRoom(user *mafia_connection.User) {
	r.mux.Lock()
	defer r.mux.Unlock()
	r.sendForAll(fmt.Sprintf("Player '%s' joined room '%d'", user.Nickname, r.ID))
	if len(r.players) == 4 {
		r.startGame()
		r.sendForAll("Game started!")
	}
}

func (r *Room) LeaveRoom(user *mafia_connection.User) {
	r.mux.Lock()
	defer r.mux.Unlock()
	r.sendForAll(fmt.Sprintf("Player '%s' left the room '%d'", user.Nickname, r.ID))
	for i := range r.players {
		if r.players[i].info.User.ID == user.ID {
			r.players = append(r.players[:i], r.players[i+1:]...)
			return
		}
	}
}

func GetNewRoom() *Room {
	return &Room{
		ID:      rand.Uint64(),
		players: make([]*Player, 0),
		mux:     sync.Mutex{},
		state:   mafia_connection.State_NOT_STARTED,
	}
}
