package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfig(t *testing.T) {
	t.Run("Default Values", func(t *testing.T) {
		cfg, err := LoadConfig()
		assert.NoError(t, err)
		assert.Equal(t, "local", cfg.AppEnv)
		assert.Equal(t, "8080", cfg.Port)
	})

	t.Run("Environment Variables", func(t *testing.T) {
		os.Setenv("PORT", "9999")
		defer os.Unsetenv("PORT")
		
		cfg, err := LoadConfig()
		assert.NoError(t, err)
		assert.Equal(t, "9999", cfg.Port)
	})
}
