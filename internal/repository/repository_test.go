package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitRedis_Fail(t *testing.T) {
	// Try to connect to non-existent redis
	client, err := InitRedis("localhost:1", "", 0)
	assert.Error(t, err)
	assert.Nil(t, client)
}
