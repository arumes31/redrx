package repository

import (
	"fmt"
	"log"
	"strings"

	"redrx/internal/config"

	"github.com/glebarez/sqlite"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func InitDB(cfg config.Config) (*gorm.DB, error) {
	var dialer gorm.Dialector
	if strings.HasPrefix(cfg.DatabaseURL, "postgres") {
		dialer = postgres.Open(cfg.DatabaseURL)
	} else if strings.HasPrefix(cfg.DatabaseURL, "sqlite") {
		dialer = sqlite.Open(strings.TrimPrefix(cfg.DatabaseURL, "sqlite://"))
	} else {
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.DatabaseURL)
	}

	db, err := gorm.Open(dialer, &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return db, nil
}

func RunMigrations(databaseURL string, sourcePath string) error {
	if sourcePath == "" {
		sourcePath = "file://migration"
	}
	m, err := migrate.New(
		sourcePath,
		databaseURL,
	)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run up migrations: %w", err)
	}

	log.Println("Database migrations ran successfully")
	return nil
}
