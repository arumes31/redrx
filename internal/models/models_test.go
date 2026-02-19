package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModels(t *testing.T) {
	t.Run("URL TableName", func(t *testing.T) {
		url := URL{}
		assert.Equal(t, "urls", url.TableName())
	})
}
