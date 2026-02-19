package services

import (
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/time/rate"
)

func TestNewIPRateLimiter(t *testing.T) {
	logger := slog.Default()
	r := rate.Limit(10)
	b := 5
	limiter := NewIPRateLimiter(r, b, logger)

	assert.NotNil(t, limiter)
	assert.Equal(t, r, limiter.r)
	assert.Equal(t, b, limiter.b)
	assert.Equal(t, logger, limiter.logger)
	assert.NotNil(t, limiter.ips)
}

func TestIPRateLimiter_GetLimiter(t *testing.T) {
	limiter := NewIPRateLimiter(rate.Limit(10), 5, slog.Default())
	ip := "192.168.1.1"

	l1 := limiter.GetLimiter(ip)
	assert.NotNil(t, l1)
	assert.Equal(t, rate.Limit(10), l1.Limit())
	assert.Equal(t, 5, l1.Burst())

	// Get again should return same limiter
	l2 := limiter.GetLimiter(ip)
	assert.Equal(t, l1, l2)

	// Different IP should return different limiter
	l3 := limiter.GetLimiter("1.1.1.1")
	assert.NotSame(t, l1, l3)
}

func TestIPRateLimiter_StartCleanup(t *testing.T) {
	limiter := NewIPRateLimiter(rate.Limit(1), 1, slog.Default())
	
	// Fill the map to trigger cleanup
	for i := 0; i < 10001; i++ {
		limiter.GetLimiter(fmt.Sprintf("ip-%d", i))
	}
	
	assert.Equal(t, 10001, len(limiter.ips))
	
	// Start cleanup with a very short interval
	limiter.StartCleanup(10 * time.Millisecond)
	
	// Wait for cleanup to run
	time.Sleep(100 * time.Millisecond)
	
	limiter.mu.RLock()
	defer limiter.mu.RUnlock()
	assert.Equal(t, 0, len(limiter.ips))
}
