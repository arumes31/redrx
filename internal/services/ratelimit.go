package services

import (
	"log/slog"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type IPRateLimiter struct {
	ips    map[string]*rate.Limiter
	mu     sync.RWMutex
	r      rate.Limit
	b      int
	logger *slog.Logger
}

func NewIPRateLimiter(r rate.Limit, b int, logger *slog.Logger) *IPRateLimiter {
	limiter := &IPRateLimiter{
		ips:    make(map[string]*rate.Limiter),
		r:      r,
		b:      b,
		logger: logger,
	}

	return limiter
}

func (i *IPRateLimiter) StartCleanup(interval time.Duration) {
	go func() {
		for {
			time.Sleep(interval)
			i.mu.Lock()
			if len(i.ips) > 10000 {
				i.logger.Info("Cleaning up rate limiter map", "count", len(i.ips))
				i.ips = make(map[string]*rate.Limiter)
			}
			i.mu.Unlock()
		}
	}()
}

func (i *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
	i.mu.Lock()
	defer i.mu.Unlock()

	limiter, exists := i.ips[ip]
	if !exists {
		limiter = rate.NewLimiter(i.r, i.b)
		i.ips[ip] = limiter
	}

	return limiter
}
