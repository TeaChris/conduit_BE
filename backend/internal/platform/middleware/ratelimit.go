package middleware

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimiter returns middleware that limits request rate per client IP.
// Uses an in-memory token bucket — suitable for single-instance deployments.
// For distributed rate limiting, swap this with a Redis-backed implementation.
func RateLimiter(rateLimit float64, burst int) gin.HandlerFunc {
	var (
		mu       sync.Mutex
		limiters = make(map[string]*rate.Limiter)
	)

	getLimiter := func(ip string) *rate.Limiter {
		mu.Lock()
		defer mu.Unlock()

		if limiter, exists := limiters[ip]; exists {
			return limiter
		}

		limiter := rate.NewLimiter(rate.Limit(rateLimit), burst)
		limiters[ip] = limiter
		return limiter
	}

	return func(c *gin.Context) {
		limiter := getLimiter(c.ClientIP())
		if !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{
					"code":    "RATE_LIMITED",
					"message": "Too many requests. Please try again later.",
				},
			})
			return
		}
		c.Next()
	}
}
