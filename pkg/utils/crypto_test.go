package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHashPassword(t *testing.T) {
	password := "my-secure-password"
	hash, err := HashPassword(password)
	
	assert.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, password, hash)
}

func TestCheckPasswordHash(t *testing.T) {
	password := "my-secure-password"
	wrongPassword := "wrong-password"
	hash, _ := HashPassword(password)

	assert.True(t, CheckPasswordHash(password, hash))
	assert.False(t, CheckPasswordHash(wrongPassword, hash))
}

func TestHashPassword_Error(t *testing.T) {
	password := make([]byte, 73)
	for i := range password {
		password[i] = 'a'
	}
	hash, err := HashPassword(string(password))
	
	assert.Error(t, err)
	assert.Empty(t, hash)
}
