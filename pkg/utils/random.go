package utils

import (
	"math/rand"
	"time"

	"github.com/google/uuid"
)

const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var seededRand *rand.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))

// GenerateShortCode generates a random string of fixed length
func GenerateShortCode(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

// GenerateAPIKey generates a UUID string to be used as an API keys
func GenerateAPIKey() string {
	return uuid.NewString()
}
