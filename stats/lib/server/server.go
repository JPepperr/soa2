package server

import (
	"context"
	"fmt"
	"log"
	"mafia/stats/lib/pdf"
	"mafia/stats/lib/storage"
	"mafia/utils"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"

	"mafia/stats/graph"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
)

type Config struct {
	UserStatsPort uint32 `config:"user-stats-port"`
	GameStatsPort uint32 `config:"game-stats-port"`
}

type Server struct {
	router  *gin.Engine
	storage *storage.Storage
	gqlsrv  *handler.Server
	ch      *amqp.Channel
	queue   amqp.Queue
}

func InitServer() *Server {
	router := gin.Default()

	storage := storage.GetNewStorage()

	srv := Server{
		router:  router,
		storage: storage,
		gqlsrv:  handler.NewDefaultServer(graph.NewExecutableSchema(graph.Config{Resolvers: &graph.Resolver{Storage: storage}})),
	}

	http.Handle("/", playground.Handler("GraphQL playground", "/query"))
	http.Handle("/query", srv.gqlsrv)

	router.POST("/user/:id", srv.UpdateUser)
	router.GET("/user/:id", srv.GetUser)
	router.DELETE("/user/:id", srv.DeleteUserData)
	router.GET("/users", srv.GetUsers)

	router.POST("/push", srv.SaveGameResult)

	router.GET("/stat/:id", srv.GeneratePdfRequest)
	router.GET("/pdf/:id", srv.GetPdf)

	return &srv
}

func (s *Server) NewPDFTask(id string, name string) error {
	return s.ch.PublishWithContext(context.Background(),
		"",           // exchange
		s.queue.Name, // routing key
		false,        // mandatory
		false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "text/plain",
			Body:         []byte(id + "#" + name),
		},
	)
}

func (s *Server) StartWorker(stopJobs chan bool, jobs *sync.WaitGroup) {
	defer jobs.Done()
	err := s.ch.Qos(
		1,     // prefetch count
		0,     // prefetch size
		false, // global
	)
	if err != nil {
		log.Fatalf("Failed to set QoS: %v", err)
	}

	msgs, err := s.ch.Consume(
		s.queue.Name, // queue
		"",           // consumer
		false,        // auto-ack
		false,        // exclusive
		false,        // no-local
		false,        // no-wait
		nil,          // args
	)
	if err != nil {
		log.Fatalf("Failed to register a consumer: %v", err)
	}
	for {
		select {
		case <-stopJobs:
			return
		case message, ok := <-msgs:
			if !ok {
				return
			}
			req := strings.Split(string(message.Body), "#")
			id := req[0]
			name := req[1]
			err := s.GeneratePdf(id, name)
			if err != nil {
				message.Ack(false)
			}
		}
	}
}

func (s *Server) Run(cfg *Config) {
	conn, err := amqp.Dial("amqp://guest:guest@rabbitmq/")
	if err != nil {
		log.Fatalf("Fail to connect to rabbitmq: %v", err)
	}
	defer conn.Close()

	s.ch, err = conn.Channel()
	if err != nil {
		log.Fatalf("Fail to open channel: %v", err)
	}
	defer s.ch.Close()

	s.queue, err = s.ch.QueueDeclare(
		"pdf", // name
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		log.Fatalf("Failed to declare a queue: %v", err)
	}

	wg := sync.WaitGroup{}
	wg.Add(2)

	stopJobs := make(chan bool)
	workers := sync.WaitGroup{}
	workers.Add(1)

	go s.StartWorker(stopJobs, &workers)

	go func() {
		defer wg.Done()
		err := http.ListenAndServe(fmt.Sprintf("[::]:%d", cfg.UserStatsPort), s.router.Handler())
		log.Fatalf("UserStats service failed: %v", err)
	}()

	go func() {
		defer wg.Done()
		err := http.ListenAndServe(fmt.Sprintf("[::]:%d", cfg.GameStatsPort), nil)
		log.Fatalf("GameStats service failed: %v", err)
	}()

	wg.Wait()
	close(stopJobs)
	workers.Wait()
}

func SendError(c *gin.Context, code int, err error) {
	c.JSON(code, gin.H{"error": err.Error()})
}

func (s *Server) GetPdf(c *gin.Context) {
	id := c.Param("id")

	data, err := s.storage.GetPdfData(id)
	if err != nil {
		SendError(c, http.StatusNotFound, err)
		return
	}

	c.Data(http.StatusOK, "application/pdf", data)
}

func (s *Server) GeneratePdfRequest(c *gin.Context) {
	id := c.Param("id")

	_, _, err := s.storage.GetUser(id)
	if err != nil {
		SendError(c, http.StatusNotFound, err)
		return
	}
	name := utils.GenerateNickname()
	err = s.NewPDFTask(id, name)
	if err != nil {
		SendError(c, http.StatusNotFound, err)
		return
	}

	c.JSON(http.StatusCreated, "http://localhost:6669/pdf/"+name)
}

func (s *Server) GeneratePdf(id string, name string) error {
	user, picData, err := s.storage.GetFullUser(id)
	if err != nil {
		return err
	}
	pdfContent, err := pdf.GeneratePDF(user, picData)
	if err != nil {
		return err
	}
	err = s.storage.SavePdf(name, pdfContent)
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) SaveGameResult(c *gin.Context) {
	var game storage.GameInfo
	if err := c.BindJSON(&game); err != nil {
		SendError(c, http.StatusBadRequest, err)
		return
	}
	if err := s.storage.SaveGameResult(&game); err != nil {
		SendError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusCreated, game)
}

func (s *Server) UpdateUser(c *gin.Context) {
	id := c.Param("id")

	user := &storage.UserR{}
	if err := c.ShouldBindWith(user, binding.FormMultipart); err != nil {
		SendError(c, http.StatusBadRequest, err)
		return
	}

	err := s.storage.UpdateUser(id, user)
	if err != nil {
		SendError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, user)
}

func (s *Server) DeleteUserData(c *gin.Context) {
	id := c.Param("id")

	err := s.storage.DeleteUserData(id)
	if err != nil {
		SendError(c, http.StatusNotFound, err)
	} else {
		c.Status(http.StatusOK)
	}
}

func (s *Server) GetUsers(c *gin.Context) {
	ids := c.DefaultQuery("ids", "")
	res := make([]*storage.UserR, 0)
	if ids != "" {
		for _, id := range strings.Split(ids, ",") {
			user, _, err := s.storage.GetUser(id)
			if err != nil {
				SendError(c, http.StatusBadRequest, err)
				return
			}
			res = append(res, user)
		}
	}
	c.JSON(http.StatusOK, res)
}

func (s *Server) GetUser(c *gin.Context) {
	id := c.Param("id")

	user, picData, err := s.storage.GetUser(id)
	if err != nil {
		SendError(c, http.StatusNotFound, err)
		return
	}

	writer := multipart.NewWriter(c.Writer)
	c.Header("Content-Type", "multipart/form-data")

	if len(picData) > 0 {
		fw, err := writer.CreateFormFile("picture", "picture")
		if err != nil {
			SendError(c, http.StatusNotFound, err)
			return
		}
		_, err = fw.Write(picData)
		if err != nil {
			SendError(c, http.StatusNotFound, err)
			return
		}
	}

	err = writer.WriteField("nickname", user.Nickname)
	if err != nil {
		SendError(c, http.StatusNotFound, err)
		return
	}
	err = writer.WriteField("sex", user.Sex)
	if err != nil {
		SendError(c, http.StatusNotFound, err)
		return
	}
	err = writer.WriteField("email", user.Email)
	if err != nil {
		SendError(c, http.StatusNotFound, err)
		return
	}

	writer.Close()
	c.Status(http.StatusOK)
}
