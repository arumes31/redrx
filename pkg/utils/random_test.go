package utils

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGenerateShortCode(t *testing.T) {
	length := 8
	code := GenerateShortCode(length)
	
	assert.Equal(t, length, len(code))
	
	// Ensure only charset characters are used
	for _, char := range code {
		assert.True(t, strings.Contains(charset, string(char)))
	}
}

func TestGenerateAPIKey(t *testing.T) {
	key := GenerateAPIKey()
	
	assert.NotEmpty(t, key)
	_, err := uuid.Parse(key)
	assert.NoError(t, err)
}
