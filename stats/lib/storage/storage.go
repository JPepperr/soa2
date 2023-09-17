package storage

import (
	"errors"
	"io"
	"mime/multipart"
	"net/textproto"
	"path"
	"sync"

	"github.com/liamg/memoryfs"
)

type UserInfo struct {
	nickname string
	sex      string
	email    string
	gamesCnt int
	wins     int
	loses    int
	duration int64
}

type Player struct {
	Nickname string `json:"nickname"`
	IsWinner bool   `json:"isWinner"`
	Role     string `json:"role"`
}

type GameInfo struct {
	Id       uint64   `json:"id"`
	Duration int64    `json:"duration"`
	Players  []Player `json:"players"`
}

type Storage struct {
	fs    *memoryfs.FS
	users map[string]*UserInfo
	games map[uint64]*GameInfo
	mux   sync.Mutex
}

func GetNewStorage() *Storage {
	fs := memoryfs.New()
	fs.MkdirAll(PICS_DIR, 0o700)
	fs.MkdirAll(PDF_DIR, 0o700)
	return &Storage{
		fs:    fs,
		users: make(map[string]*UserInfo),
		games: make(map[uint64]*GameInfo),
		mux:   sync.Mutex{},
	}
}

type UserR struct {
	Nickname string                `form:"nickname" json:"nickname"`
	Sex      string                `form:"sex" json:"sex"`
	Email    string                `form:"email" json:"email"`
	Picture  *multipart.FileHeader `form:"picture" json:"picture"`
}

func (s *Storage) getOrCreateUser(nickname string) *UserInfo {
	user, ok := s.users[nickname]
	if ok {
		return user
	} else {
		s.users[nickname] = &UserInfo{
			nickname: nickname,
		}
		return s.users[nickname]
	}
}

func getPathForNickcname(nickname string) string {
	return path.Join(PICS_DIR, nickname)
}

func (s *Storage) SaveGameResult(game *GameInfo) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	_, ok := s.games[game.Id]
	if ok {
		return errAlreadyExistingGame
	}

	s.games[game.Id] = game
	for _, player := range game.Players {
		user := s.getOrCreateUser(player.Nickname)
		user.gamesCnt++
		if player.IsWinner {
			user.wins++
		} else {
			user.loses++
		}
		user.duration += game.Duration
	}

	return nil
}

func (s *Storage) UpdateUser(nickname string, user *UserR) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	curUser := s.getOrCreateUser(nickname)

	curUser.nickname = user.Nickname
	curUser.sex = user.Sex
	curUser.email = user.Email

	if user.Picture != nil {
		file, err := user.Picture.Open()
		if err != nil {
			return err
		}
		bytes, err := io.ReadAll(file)
		if err != nil {
			return err
		}
		if err := s.fs.WriteFile(getPathForNickcname(nickname), bytes, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (s *Storage) DeleteUserData(nickname string) error {
	s.mux.Lock()
	defer s.mux.Unlock()

	user, ok := s.users[nickname]
	if !ok || user.nickname != nickname {
		return errNotFound
	}
	user.nickname = ""
	user.email = ""
	user.sex = ""
	s.fs.Remove(getPathForNickcname(nickname))
	return nil
}

func (s *Storage) GetUser(nickname string) (*UserR, []byte, error) {
	s.mux.Lock()
	defer s.mux.Unlock()

	data := make([]byte, 0)

	user, ok := s.users[nickname]
	if !ok || user.nickname != nickname {
		return nil, data, errNotFound
	}
	header, data := s.createFileHeader(getPathForNickcname(nickname))
	if header == nil {
		return nil, data, errFSError
	}

	return &UserR{
		Nickname: user.nickname,
		Sex:      user.sex,
		Email:    user.email,
		Picture:  header,
	}, data, nil
}

func (s *Storage) createFileHeader(fileName string) (*multipart.FileHeader, []byte) {
	bytes := make([]byte, 0)
	file, err := s.fs.Open(fileName)
	if err != nil {
		return nil, bytes
	}
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, bytes
	}
	bytes, err = io.ReadAll(file)
	if err != nil {
		return nil, bytes
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="picture"`)
	return &multipart.FileHeader{
		Filename: fileInfo.Name(),
		Header:   header,
		Size:     fileInfo.Size(),
	}, bytes
}

const (
	PICS_DIR = "data/pics"
	PDF_DIR  = "data/pdf"
)

var (
	errNotFound            = errors.New("user not found")
	errAlreadyExistingGame = errors.New("game already exist")
	errFSError             = errors.New("internal FS error")
)
