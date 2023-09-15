package game

import (
	"fmt"
	mafia_connection "mafia/protos"
	"mafia/utils"
	"math/rand"
	"sync"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

type Player struct {
	voteFor         int
	checkedBySherif bool
	shownBySherif   bool
	connection      mafia_connection.MafiaService_RouteGameServer
	info            mafia_connection.Player
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
			voteFor:         -1,
			checkedBySherif: false,
			shownBySherif:   false,
			connection:      stream,
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
			if r.players[i].shownBySherif {
				rightRole = r.players[i].info.Role
			}
			if r.players[i].checkedBySherif && role == mafia_connection.Role_SHERIFF {
				rightRole = r.players[i].info.Role
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

func (r *Room) sendMessageForUser(user *mafia_connection.User, message string) {
	for _, player := range r.players {
		if player.info.User.ID == user.ID {
			player.connection.Send(&mafia_connection.ServerAction{
				Action: &mafia_connection.ServerAction_Event{
					Event: &mafia_connection.RoomEvent{
						Event:    wrapperspb.String(message),
						RoomInfo: r.getRoomInfoForPlayer(player.info.User.ID),
					},
				},
			})
			break
		}
	}
}

func (r *Room) SendMessageForUser(user *mafia_connection.User, message string) {
	r.mux.Lock()
	defer r.mux.Unlock()
	r.sendMessageForUser(user, message)
}

func (r *Room) changeStateAfterVotes() {
	cntMafia := 0
	cntNotMafia := 0
	for _, p := range r.players {
		if !p.info.Alive {
			continue
		}
		if p.info.Role == mafia_connection.Role_MAFIA {
			cntMafia++
		} else {
			cntNotMafia++
		}
	}
	if cntMafia == 0 {
		r.changeState(mafia_connection.State_END)
		r.sendForAll("Civilians won")
		return
	}
	if cntMafia == cntNotMafia {
		r.changeState(mafia_connection.State_END)
		r.sendForAll("Mafia won")
		return
	}
	if r.state == mafia_connection.State_DAY {
		r.changeState(mafia_connection.State_NIGHT)
		r.sendForAll("Night started")
	} else {
		r.changeState(mafia_connection.State_DAY)
		r.sendForAll("Night ended")
	}
}

func (r *Room) checkAllVoted() {
	var message string
	if r.state == mafia_connection.State_NIGHT {
		disclosureRequest := make([]int, len(r.players))
		killRequest := make([]int, len(r.players))

		for _, p := range r.players {
			if p.info.Role == mafia_connection.Role_MAFIA && p.info.Alive {
				if p.voteFor == -1 {
					return
				}
				killRequest[p.voteFor] += 1
			}
		}
		killed := r.players[utils.GetRandomMaximumIndex(killRequest)]
		killed.info.Alive = false

		for _, p := range r.players {
			if p.info.Role == mafia_connection.Role_SHERIFF && p.info.Alive {
				if p.voteFor == -1 {
					return
				}
				disclosureRequest[p.voteFor] += 1
			}
		}
		disclosured := r.players[utils.GetRandomMaximumIndex(disclosureRequest)]
		disclosured.checkedBySherif = true

		message = fmt.Sprintf("Mafia killed '%s' that night", killed.info.User.Nickname)

		for _, p := range r.players {
			if p.info.Role == mafia_connection.Role_SHERIFF {
				r.sendInfoForUser(p.info.User, fmt.Sprintf(
					"'%s' is %s",
					disclosured.info.User.Nickname,
					disclosured.info.Role.String(),
				))
			}
		}
	} else {
		voteRequest := make([]int, len(r.players))
		for _, p := range r.players {
			if p.info.Alive {
				if p.voteFor == -1 {
					return
				}
				voteRequest[p.voteFor] += 1
			}
		}
		votedOut := r.players[utils.GetRandomMaximumIndex(voteRequest)]
		votedOut.info.Alive = false
		message = fmt.Sprintf("The city voted out '%s'", votedOut.info.User.Nickname)
	}
	r.sendInfoForAll(message)
	r.changeStateAfterVotes()
}

func (r *Room) VoteRequest(author *mafia_connection.User, target *mafia_connection.User) {
	r.mux.Lock()
	defer r.mux.Unlock()
	var authorPlayer, targetPlayer *Player
	targetId := -1
	for i, p := range r.players {
		if p.info.User.ID == author.ID {
			authorPlayer = p
		}
		if p.info.User.ID == target.ID {
			targetPlayer = p
			targetId = i
		}
	}
	if !authorPlayer.info.Alive || !targetPlayer.info.Alive {
		r.SendIncorrectRequestMessage(author)
		return
	}
	if r.state == mafia_connection.State_END || r.state == mafia_connection.State_NOT_STARTED {
		r.SendIncorrectRequestMessage(author)
		return
	}
	if r.state == mafia_connection.State_NIGHT {
		if authorPlayer.info.Role == mafia_connection.Role_CIVILIAN || authorPlayer.info.Role == mafia_connection.Role_UNKNOWN {
			r.SendIncorrectRequestMessage(author)
			return
		}
	}
	authorPlayer.voteFor = targetId
	r.checkAllVoted()
}

func (r *Room) ShowRequest(author *mafia_connection.User, target *mafia_connection.User) {
	r.mux.Lock()
	defer r.mux.Unlock()
	var authorPlayer, targetPlayer *Player
	for _, p := range r.players {
		if p.info.User.ID == author.ID {
			authorPlayer = p
		}
		if p.info.User.ID == target.ID {
			targetPlayer = p
		}
	}
	if !authorPlayer.info.Alive || authorPlayer.info.Role != mafia_connection.Role_SHERIFF {
		r.SendIncorrectRequestMessage(author)
		return
	}
	if r.state != mafia_connection.State_DAY {
		r.SendIncorrectRequestMessage(author)
		return
	}
	targetPlayer.shownBySherif = true
	r.sendForAll(fmt.Sprintf(
		"Sheriff '%s' checked '%s' at night and exposes that he is a mafia",
		author.Nickname,
		target.Nickname,
	))
}

func (r *Room) sendForAll(message string) {
	for _, player := range r.players {
		player.connection.Send(&mafia_connection.ServerAction{
			Action: &mafia_connection.ServerAction_Event{
				Event: &mafia_connection.RoomEvent{
					Event:    wrapperspb.String(message),
					RoomInfo: r.getRoomInfoForPlayer(player.info.User.ID),
				},
			},
		})
	}
}

func (r *Room) changeState(newState mafia_connection.State) {
	r.state = newState
	for _, p := range r.players {
		p.voteFor = -1
	}
}

func (r *Room) startGame() {
	r.changeState(mafia_connection.State_NIGHT)
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

func (r *Room) sendInfoForAll(message string) {
	for _, player := range r.players {
		player.connection.Send(&mafia_connection.ServerAction{
			Action: &mafia_connection.ServerAction_ServerMessage{
				ServerMessage: message,
			},
		})
	}
}

func (r *Room) sendInfoForUser(user *mafia_connection.User, message string) {
	for _, player := range r.players {
		if player.info.User.ID == user.ID {
			player.connection.Send(&mafia_connection.ServerAction{
				Action: &mafia_connection.ServerAction_ServerMessage{
					ServerMessage: message,
				},
			})
			break
		}
	}
}

func (r *Room) SendInfoForUser(user *mafia_connection.User, message string) {
	r.mux.Lock()
	defer r.mux.Unlock()
	r.sendInfoForUser(user, message)
}

func (r *Room) SendIncorrectRequestMessage(user *mafia_connection.User) {
	r.SendInfoForUser(user, "Incorrect command")
}
