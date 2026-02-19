package repository

import (
	"redrx/internal/config"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitDB(t *testing.T) {
	t.Run("SQLite Success", func(t *testing.T) {
		cfg := config.Config{
			DatabaseURL: "sqlite://:memory:",
		}
		db, err := InitDB(cfg)
		assert.NoError(t, err)
		assert.NotNil(t, db)
	})

	t.Run("Unsupported Driver", func(t *testing.T) {
		cfg := config.Config{
			DatabaseURL: "mysql://localhost",
		}
		_, err := InitDB(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported database driver")
	})

	t.Run("Invalid SQLite Path", func(t *testing.T) {
		cfg := config.Config{
			DatabaseURL: "sqlite:///non/existent/path/db.sqlite",
		}
		_, err := InitDB(cfg)
		assert.Error(t, err)
	})

	t.Run("Postgres Invalid URL", func(t *testing.T) {
		cfg := config.Config{
			DatabaseURL: "postgres://invalid:invalid@localhost:5432/db",
		}
		_, err := InitDB(cfg)
		assert.Error(t, err)
	})
}

func TestRunMigrations_Fail(t *testing.T) {
	t.Run("Invalid Source Path", func(t *testing.T) {
		err := RunMigrations("postgres://localhost", "file://non-existent")
		assert.Error(t, err)
	})

	t.Run("Unsupported DB Driver", func(t *testing.T) {
		err := RunMigrations("mysql://localhost", "")
		assert.Error(t, err)
	})

	t.Run("Empty Database URL", func(t *testing.T) {
		err := RunMigrations("", "")
		assert.Error(t, err)
	})
}
