package services

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"redrx/internal/models"

	"github.com/stretchr/testify/assert"
)

func TestAuditService(t *testing.T) {
	db := setupTestDB()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewAuditService(db, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go service.Start(ctx)

	t.Run("Log Action", func(t *testing.T) {
		userID := uint(1)
		service.LogAction(&userID, "TEST_ACTION", "entity_1", map[string]string{"foo": "bar"}, "127.0.0.1")

		// Wait for worker to process
		time.Sleep(100 * time.Millisecond)

		var log models.AuditLog
		err := db.First(&log).Error
		assert.NoError(t, err)
		assert.Equal(t, "TEST_ACTION", log.Action)
		assert.Equal(t, "entity_1", log.EntityID)
		assert.Contains(t, log.Details, "foo")
	})
}
