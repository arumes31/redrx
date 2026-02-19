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

	t.Run("Channel Full", func(t *testing.T) {
		service := NewAuditService(db, logger)
		// Fill channel
		for i := 0; i < 100; i++ {
			service.LogAction(nil, "ACTION", "ID", nil, "IP")
		}
		// Should drop
		service.LogAction(nil, "DROP", "ID", nil, "IP")
	})

	t.Run("DB Error", func(t *testing.T) {
		dbErr := setupTestDB()
		dbErr.Migrator().DropTable(&models.AuditLog{})
		serviceErr := NewAuditService(dbErr, logger)
		
		ctxErr, cancelErr := context.WithCancel(context.Background())
		go serviceErr.Start(ctxErr)
		
		serviceErr.LogAction(nil, "ERROR", "ID", nil, "IP")
		time.Sleep(100 * time.Millisecond)
		cancelErr()
	})
}
