package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mafia/client/lib/cli"
	mafia_connection "mafia/protos"
	"mafia/utils"
	"math/rand"
	"strings"
	"sync"

	"github.com/c-bata/go-prompt"
	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Config struct {
	ServerAddr    string `config:"server-addr"`
	RabbitmqCreds string `config:"rabbitmq-creds"`
}

type Option struct {
	chat   bool
	rand   bool
	action *mafia_connection.PlayerAction
}

type Client struct {
	nickname string

	possibleOptions map[string]Option
	roomInfo        *mafia_connection.RoomInfo

	mux        sync.Mutex
	chat       *ClientChat
	cli        *cli.Cli
	prompt     *prompt.Prompt
	grpcClient mafia_connection.MafiaServiceClient
	stream     mafia_connection.MafiaService_RouteGameClient
}

func (c *Client) Clear() {
	for k := range c.possibleOptions {
		delete(c.possibleOptions, k)
	}
	c.roomInfo = nil
	c.chat = createChat()
	cl, p := cli.GetCli()
	c.cli = cl
	c.prompt = p
}

func GetClient() *Client {
	c, p := cli.GetCli()
	return &Client{
		nickname:        utils.GenerateNickname(),
		possibleOptions: make(map[string]Option),
		roomInfo:        nil,
		mux:             sync.Mutex{},
		chat:            createChat(),
		cli:             c,
		prompt:          p,
	}
}

func (c *Client) getMyId() int {
	for id := range c.roomInfo.Players {
		if c.roomInfo.Players[id].User.Nickname == c.nickname {
			return id
		}
	}
	return -1
}

func (c *Client) printRoomInfo() {
	if c.roomInfo == nil {
		return
	}
	border := strings.Repeat("-", 75)
	roomInfo := []string{border}
	roomInfo = append(roomInfo, fmt.Sprintf(
		"Room: '%d', State: %s, Ctrl + D to leave",
		c.roomInfo.RoomID,
		c.roomInfo.State.String(),
	))
	myId := c.getMyId()
	for id, player := range c.roomInfo.Players {
		isMe := ""
		if id == myId {
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
	for _, s := range roomInfo {
		c.cli.Println(s)
	}
}

func (c *Client) addRandOption() {
	for _, opt := range c.possibleOptions {
		if !opt.chat {
			c.possibleOptions[RAND_COMMAND] = Option{rand: true}
			return
		}
	}
}

func (c *Client) buildOptions() {
	defer c.addRandOption()
	for k := range c.possibleOptions {
		delete(c.possibleOptions, k)
	}
	self := c.roomInfo.Players[c.getMyId()]
	if c.roomInfo.State == mafia_connection.State_END {
		c.possibleOptions[CHAT_COMMAND] = Option{chat: true}
		return
	}
	if !self.Alive {
		return
	}
	if c.roomInfo.State == mafia_connection.State_NIGHT {
		if self.Role != mafia_connection.Role_CIVILIAN {
			c.possibleOptions[CHAT_COMMAND] = Option{chat: true}
			voteOpt := ""
			if self.Role == mafia_connection.Role_MAFIA {
				voteOpt = "kill "
			} else {
				voteOpt = "check "
			}
			for _, p := range c.roomInfo.Players {
				if p.Alive {
					c.possibleOptions[voteOpt+p.User.Nickname] = Option{
						chat: false,
						action: &mafia_connection.PlayerAction{
							Action: &mafia_connection.PlayerAction_Vote{
								Vote: p.User,
							},
						},
					}
				}
			}
		}
		return
	}
	c.possibleOptions[CHAT_COMMAND] = Option{chat: true}
	if c.roomInfo.State == mafia_connection.State_DAY {
		if self.Role == mafia_connection.Role_SHERIFF {
			for _, p := range c.roomInfo.Players {
				if p.Alive && p.Role == mafia_connection.Role_MAFIA {
					c.possibleOptions["show "+p.User.Nickname] = Option{
						chat: false,
						action: &mafia_connection.PlayerAction{
							Action: &mafia_connection.PlayerAction_Show{
								Show: p.User,
							},
						},
					}
				}
			}
		}
		for _, p := range c.roomInfo.Players {
			if p.Alive {
				c.possibleOptions["vote "+p.User.Nickname] = Option{
					chat: false,
					action: &mafia_connection.PlayerAction{
						Action: &mafia_connection.PlayerAction_Vote{
							Vote: p.User,
						},
					},
				}
			}
		}
	}

}

func (c *Client) buildOptionsAndSuggests() {
	c.buildOptions()
	c.cli.Suggests = make([]prompt.Suggest, 0)
	for text, opt := range c.possibleOptions {
		if opt.rand {
			c.cli.Suggests = append(c.cli.Suggests, prompt.Suggest{
				Text: text,
			})
		}
	}
	for text, opt := range c.possibleOptions {
		if !opt.rand {
			c.cli.Suggests = append(c.cli.Suggests, prompt.Suggest{
				Text: text,
			})
		}
	}
}

func (c *Client) ResolveAction(action *mafia_connection.ServerAction) error {
	c.mux.Lock()
	defer c.mux.Unlock()
	switch {
	case action.GetServerMessage() != "":
		c.cli.Println(action.GetServerMessage())
	case action.GetEvent() != nil:
		event := action.GetEvent()
		addChats := false
		if c.roomInfo != nil &&
			c.roomInfo.State == mafia_connection.State_NOT_STARTED &&
			event.RoomInfo.State != mafia_connection.State_NOT_STARTED {
			addChats = true
		}
		c.roomInfo = event.RoomInfo
		if addChats {
			c.addRoleChat()
		}
		c.buildOptionsAndSuggests()
		if event.Event != nil {
			c.cli.Println(event.Event.Value)
		}
		c.printRoomInfo()
	}
	return nil
}

func (c *Client) ResolveCommand(command string) error {
	c.mux.Lock()
	defer c.mux.Unlock()
	if strings.HasPrefix(command, CHAT_COMMAND) {
		_, ok := c.possibleOptions[CHAT_COMMAND]
		if !ok {
			c.cli.Println(UNKNOWN_COMMAND)
			return errUnknownCommand
		}
		return c.sendMessage(command[len(CHAT_COMMAND):])
	}
	opt, ok := c.possibleOptions[command]
	if !ok {
		c.cli.Println(UNKNOWN_COMMAND)
		return errUnknownCommand
	}
	if opt.rand {
		cnt := rand.Intn(len(c.possibleOptions) - 2)
		for desc, opt := range c.possibleOptions {
			if !opt.chat && !opt.rand {
				if cnt > 0 {
					cnt--
				} else {
					c.cli.Println(desc)
					return c.stream.Send(opt.action)
				}
			}
		}
		return nil
	} else {
		return c.stream.Send(opt.action)
	}
}

func (c *Client) HandleServerActions(
	stopJobs chan bool,
	jobs *sync.WaitGroup,
) error {
	defer jobs.Done()
	for {
		select {
		case <-stopJobs:
			return nil
		default:
			serverAction, err := c.stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			c.ResolveAction(serverAction)
		}
	}
}

func (c *Client) HandleCLIActions(
	stopJobs chan bool,
	jobs *sync.WaitGroup,
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
			c.ResolveCommand(command)
		}
	}
}

func (c *Client) HandleChat(
	stopJobs chan bool,
	jobs *sync.WaitGroup,
) {
	defer jobs.Done()
	for {
		select {
		case <-stopJobs:
			return
		case message, ok := <-c.chat.msgs:
			if !ok {
				return
			}
			c.cli.Println(string(message.Body))
		}
	}
}

func (c *Client) BeforeConnection() error {
	for {
		choice, err := cli.Choice("Select option (Ctrl + C to exit)", []string{CHANGE_NICKNAME_OPTION, CONNECT_TO_SERVER_OPTION})
		if err != nil {
			return err
		}
		if choice == CONNECT_TO_SERVER_OPTION {
			return nil
		}
		c.nickname, err = cli.Input("Enter new nickname", c.nickname, utils.ValidateNickname, utils.GetErrorMessageForNickname)
		if err != nil {
			return err
		}
	}
}

func (c *Client) Run(addr string, rabbitMqCreds string) {
	err := c.BeforeConnection()
	if err != nil {
		log.Fatalf("Internal error: %v", err)
	}

	chatConn, err := amqp.Dial(rabbitMqCreds)
	if err != nil {
		log.Fatalf("Fail to connect to rabbitmq: %v", err)
	}
	defer chatConn.Close()

	c.chat.ch, err = chatConn.Channel()
	if err != nil {
		log.Fatalf("Fail to open channel: %v", err)
	}
	defer c.chat.ch.Close()

	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials())) // TODO: Add TLS
	if err != nil {
		log.Fatalf("Fail to connect server: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())

	c.grpcClient = mafia_connection.NewMafiaServiceClient(conn)
	c.stream, err = c.grpcClient.RouteGame(ctx)
	if err != nil {
		log.Fatalf("RouteGame failed: %v", err)
	}

	init := &mafia_connection.PlayerAction{
		Action: &mafia_connection.PlayerAction_Connetion{
			Connetion: &mafia_connection.User{
				ID:       utils.NicknameHash(c.nickname),
				Nickname: c.nickname,
			},
		},
	}
	if err := c.stream.Send(init); err != nil {
		log.Fatalf("Failed to send init request to server: %v", err)
	}
	serverAction, err := c.stream.Recv()
	if err != nil {
		log.Fatalf("Failed to get init response from server: %v", err)
		cancel()
		return
	}
	c.ResolveAction(serverAction)

	err = c.initChat()
	if err != nil {
		log.Fatalf("Failed to init chat: %v", err)
		cancel()
		return
	}

	stopJobs := make(chan bool)
	var jobs sync.WaitGroup
	jobs.Add(3)
	defer jobs.Wait()

	go c.HandleCLIActions(stopJobs, &jobs)
	go c.HandleChat(stopJobs, &jobs)
	go c.HandleServerActions(stopJobs, &jobs)
	c.prompt.Run()
	cancel()
	close(stopJobs)
}

var (
	CONNECT_TO_SERVER_OPTION = "Connect to server"
	CHANGE_NICKNAME_OPTION   = "Change nickname"
	CHAT_COMMAND             = "chat "
	RAND_COMMAND             = "rand"
	UNKNOWN_COMMAND          = "Unknown command"
	errUnknownCommand        = errors.New(UNKNOWN_COMMAND)
)
