package client

import (
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"mafia/client/lib/cli"
	mafia_connection "mafia/protos"
	"math/rand"
	"strings"
	"sync"
	"unicode"

	"github.com/c-bata/go-prompt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Config struct {
	ServerAddr string `config:"server-addr"`
	BotMode    bool   `config:"bot"`
}

type Client struct {
	nickname string

	roomInfo *mafia_connection.RoomInfo

	cli        *cli.Cli
	prompt     *prompt.Prompt
	grpcClient mafia_connection.MafiaServiceClient
}

func GetClient() *Client {
	c, p := cli.GetCli()
	return &Client{
		nickname: GenerateNickname(),
		roomInfo: nil,
		cli:      c,
		prompt:   p,
	}
}

func (c *Client) PrintRoomInfo() {
	if c.roomInfo == nil {
		return
	}
	border := strings.Repeat("-", 75)
	roomInfo := []string{border}
	roomInfo = append(roomInfo, fmt.Sprintf("Room: '%d', State: %s", c.roomInfo.RoomID, c.roomInfo.State.String()))
	for _, player := range c.roomInfo.Players {
		isMe := ""
		if player.User.Nickname == c.nickname {
			isMe = "(You)"
		}
		status := "alive"
		if !player.Alive {
			status = "ghost"
		}
		roomInfo = append(roomInfo, fmt.Sprintf(
			"%s%s (Role: %s, Status: %s)",
			isMe,
			player.User.Nickname,
			player.Role.String(),
			status,
		))
	}
	roomInfo = append(roomInfo, border)
	c.cli.Println(strings.Join(roomInfo, "\n"))
}

func (c *Client) HandleServerActions(
	stopJobs chan bool,
	jobs *sync.WaitGroup,
	stream mafia_connection.MafiaService_RouteGameClient,
) {
	defer jobs.Done()
	for {
		select {
		case <-stopJobs:
			return
		default:
			serverAction, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				log.Fatalf("Broken channel: %v", err)
			}
			switch {
			case serverAction.GetChatMessage() != nil:
				text := serverAction.GetChatMessage().Text
				author := serverAction.GetChatMessage().Author.Nickname
				c.cli.Println(fmt.Sprintf("<%s> %s", author, text))
			case serverAction.GetEvent() != nil:
				event := serverAction.GetEvent()
				c.roomInfo = event.RoomInfo
				if event.Event != nil {
					c.cli.Println(event.Event.Value)
				}
				c.PrintRoomInfo()
			}
		}
	}
}

func (c *Client) HandleCLIActions(
	stopJobs chan bool,
	jobs *sync.WaitGroup,
	stream mafia_connection.MafiaService_RouteGameClient,
) {
	defer jobs.Done()
	for {
		select {
		case <-stopJobs:
			return
		case command, ok := <-c.cli.Commands:
			if !ok {
				return
			}
			c.cli.Println(command)
		}
	}
}

func NicknameHash(nick string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(nick))
	return h.Sum64()
}

func GenerateNickname() string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	l := rand.Intn(12) + 4
	b := make([]byte, l)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func ValidateNicknameImpl(nick string) string {
	if len(nick) < 4 || len(nick) > 15 {
		return "Nickname must be at least 4 characters and no longer than 15"
	}
	for _, l := range nick {
		if !unicode.IsLetter(l) {
			return "Nickname must contain only letters"
		}
	}
	return ""
}

func ValidateNickname(nick string) bool {
	return ValidateNicknameImpl(nick) == ""
}

func GetErrorMessageForNickname(nick string) string {
	return ValidateNicknameImpl(nick)
}

func (c *Client) BeforeConnection() error {
	for {
		choice, err := cli.Choice("Select option", []string{CHANGE_NICKNAME_OPTION, CONNECT_TO_SERVER_OPTION})
		if err != nil {
			return err
		}
		if choice == CONNECT_TO_SERVER_OPTION {
			return nil
		}
		c.nickname, err = cli.Input("Enter new nickname", c.nickname, ValidateNickname, GetErrorMessageForNickname)
		if err != nil {
			return err
		}
	}
}

func (c *Client) Run(addr string) {
	err := c.BeforeConnection()
	if err != nil {
		log.Fatalf("Internal error: %v", err)
	}

	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials())) // TODO: Add TLS
	if err != nil {
		log.Fatalf("Fail to dial: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())

	c.grpcClient = mafia_connection.NewMafiaServiceClient(conn)
	stream, err := c.grpcClient.RouteGame(ctx)
	if err != nil {
		log.Fatalf("RouteGame failed: %v", err)
	}

	init := &mafia_connection.PlayerAction{
		Action: &mafia_connection.PlayerAction_Connetion{
			Connetion: &mafia_connection.User{
				ID:       NicknameHash(c.nickname),
				Nickname: c.nickname,
			},
		},
	}
	if err := stream.Send(init); err != nil {
		log.Fatalf("Failed to init connection with server: %v", err)
	}

	stopJobs := make(chan bool)
	var jobs sync.WaitGroup
	jobs.Add(2)
	defer jobs.Wait()

	go c.HandleCLIActions(stopJobs, &jobs, stream)
	go c.HandleServerActions(stopJobs, &jobs, stream)
	c.prompt.Run()
	cancel()
	close(stopJobs)
}

var (
	CONNECT_TO_SERVER_OPTION = "Connect to server"
	CHANGE_NICKNAME_OPTION   = "Change nickname"
)
