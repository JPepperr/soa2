package utils

import (
	"hash/fnv"
	"math/rand"
	"unicode"
)

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

func GetRandomMaximumIndex(arr []int) int {
	max := arr[0]
	maxIds := make([]int, 0)
	for i, val := range arr {
		if val > max {
			max = val
			maxIds = make([]int, 0)
		}
		if val == max {
			maxIds = append(maxIds, i)
		}
	}
	return maxIds[rand.Intn(len(maxIds))]
}
