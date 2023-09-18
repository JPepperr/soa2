package model

import (
	"mafia/stats/lib/storage"
	"strconv"
)

func ConvertGameInfo(game *storage.GameInfo) *Game {
	users := make([]*User, 0)
	for _, player := range game.Players {
		users = append(users, &User{
			Nickname: player.Nickname,
			IsWinner: player.IsWinner,
			Role:     player.Role,
		})
	}
	return &Game{
		ID:       strconv.FormatUint(game.Id, 10),
		User:     users,
		Comments: game.Comments,
	}
}
