package server

import (
	"fmt"
	"mafia/stats/lib/storage"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type Config struct {
	Port uint32 `config:"port"`
}

type Server struct {
	router  *gin.Engine
	storage *storage.Storage
}

func InitServer() *Server {
	router := gin.Default()

	srv := Server{
		router:  router,
		storage: storage.GetNewStorage(),
	}

	router.POST("/user/:id", srv.UpdateUser)
	router.GET("/user/:id", srv.GetUser)
	router.DELETE("/user/:id", srv.DeleteUserData)
	router.GET("/users", srv.GetUsers)

	return &srv
}

func (s *Server) Run(cfg *Config) {
	addr := fmt.Sprintf("[::]:%d", cfg.Port)
	s.router.Run(addr)
}

func SendError(c *gin.Context, code int, err error) {
	c.JSON(code, gin.H{"error": err.Error()})
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
