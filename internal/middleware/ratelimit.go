package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// IPRateLimiter manages rate limiters for each IP
type IPRateLimiter struct {
	ips map[string]*rate.Limiter
	mu  sync.RWMutex
	r   rate.Limit
	b   int
}

// NewIPRateLimiter creates a new limiter with rate r and burst b
func NewIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
	i := &IPRateLimiter{
		ips: make(map[string]*rate.Limiter),
		r:   r,
		b:   b,
	}

	// Cleanup routine to remove old IPs (simple version)
	go func() {
		for {
			time.Sleep(10 * time.Minute)
			i.mu.Lock()
			// In a real app, we'd track last seen time and remove only old ones.
			// For simplicity/dev, we clear map periodically or implemented a TTL map.
			// Here we just reset to avoid unbounded growth in long running process
			if len(i.ips) > 10000 {
				i.ips = make(map[string]*rate.Limiter)
			}
			i.mu.Unlock()
		}
	}()

	return i
}

// AddIP adds an IP to the map
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

var limiter = NewIPRateLimiter(5, 10) // 5 req/s, burst 10

func RateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. Get IP, preferring Cloudflare header
		ip := c.GetHeader("CF-Connecting-IP")
		if ip == "" {
			ip = c.GetHeader("X-Forwarded-For")
		}
		if ip == "" {
			ip = c.ClientIP()
		}

		// Normalize IP (remove port if present)
		if strings.Contains(ip, ":") && !strings.Contains(ip, "]") { // IPv4 with port
			ip = strings.Split(ip, ":")[0]
		}
		// IPv6 logic handles standard ClientIP, but raw header might vary. Keeping simple.

		// 2. Check Rate Limit
		l := limiter.GetLimiter(ip)
		if !l.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "Rate limit exceeded. Please try again later.",
			})
			return
		}

		c.Next()
	}
}
