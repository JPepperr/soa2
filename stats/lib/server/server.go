package server

import (
	"fmt"
	"mafia/stats/lib/storage"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"

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

	return &srv
}

func (s *Server) Run(cfg *Config) {
	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		_ = http.ListenAndServe(fmt.Sprintf("[::]:%d", cfg.UserStatsPort), s.router.Handler())
	}()

	go func() {
		defer wg.Done()
		err := http.ListenAndServe(fmt.Sprintf("[::]:%d", cfg.GameStatsPort), nil)
		if err != nil {
			fmt.Println(err.Error())
		}
	}()

	wg.Wait()
}

func SendError(c *gin.Context, code int, err error) {
	c.JSON(code, gin.H{"error": err.Error()})
}

func (s *Server) SaveGameResult(c *gin.Context) {
	var game storage.GameInfo
	if err := c.BindJSON(&game); err != nil {
		SendError(c, http.StatusBadRequest, err)
		return
	}
	fmt.Println(game)
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
