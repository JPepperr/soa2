package client

import (
	"context"
	"fmt"
	mafia_connection "mafia/protos"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type ClientChat struct {
	ch    *amqp.Channel
	queue *amqp.Queue
	msgs  <-chan amqp.Delivery
}

func createChat() *ClientChat {
	return &ClientChat{
		ch:    nil,
		queue: nil,
	}
}

func (c *Client) getRoleExchangeName() string {
	self := c.roomInfo.Players[c.getMyId()]
	if self.Role != mafia_connection.Role_CIVILIAN && self.Role != mafia_connection.Role_UNKNOWN {
		return strconv.FormatUint(c.roomInfo.RoomID, 10) + self.Role.String()
	} else {
		return ""
	}
}

func (c *Client) getCommonExchangeName() string {
	return strconv.FormatUint(c.roomInfo.RoomID, 10)
}

func (c *Client) initChat() error {
	q, err := c.chat.ch.QueueDeclare(
		"",    // name
		false, // durable
		false, // delete when unused
		true,  // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return err
	}
	c.chat.queue = &q

	err = c.chat.ch.ExchangeDeclare(
		c.getCommonExchangeName(), // name
		"fanout",                  // type
		true,                      // durable
		false,                     // auto-deleted
		false,                     // internal
		false,                     // no-wait
		nil,                       // arguments
	)
	if err != nil {
		return err
	}

	err = c.chat.ch.QueueBind(
		q.Name,                    // queue name
		"",                        // routing key
		c.getCommonExchangeName(), // exchange
		false,
		nil,
	)
	if err != nil {
		return err
	}

	c.chat.msgs, err = c.chat.ch.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)

	if err != nil {
		return err
	}

	return nil
}

func (c *Client) addRoleChat() error {
	roleChat := c.getRoleExchangeName()
	if roleChat == "" {
		return nil
	}
	err := c.chat.ch.ExchangeDeclare(
		roleChat, // name
		"fanout", // type
		true,     // durable
		false,    // auto-deleted
		false,    // internal
		false,    // no-wait
		nil,      // arguments
	)
	if err != nil {
		return err
	}

	err = c.chat.ch.QueueBind(
		c.chat.queue.Name, // queue name
		"",                // routing key
		roleChat,          // exchange
		false,
		nil,
	)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) sendMessage(message string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	body := fmt.Sprintf("<%s> %s", c.nickname, message)

	var err error

	if c.roomInfo.State == mafia_connection.State_NIGHT {
		err = c.chat.ch.PublishWithContext(ctx,
			c.getRoleExchangeName(), // exchange
			"",                      // routing key
			false,                   // mandatory
			false,                   // immediate
			amqp.Publishing{
				ContentType: "text/plain",
				Body:        []byte(body),
			},
		)
	} else {
		err = c.chat.ch.PublishWithContext(ctx,
			c.getCommonExchangeName(), // exchange
			"",                        // routing key
			false,                     // mandatory
			false,                     // immediate
			amqp.Publishing{
				ContentType: "text/plain",
				Body:        []byte(body),
			},
		)
	}

	if err != nil {
		return err
	}

	return nil
}
