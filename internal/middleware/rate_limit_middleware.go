package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Refliqx/backend-eletter/internal/config"
	"github.com/Refliqx/backend-eletter/internal/response"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RedisRateLimiter provides Redis-based rate limiting for distributed systems
type RedisRateLimiter struct {
	client      *redis.Client
	maxAttempts int
	window      time.Duration
}

// NewRedisRateLimiter creates a new Redis-based rate limiter
func NewRedisRateLimiter(cfg *config.Config) *RedisRateLimiter {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", cfg.Redis.Host, cfg.Redis.Port),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	return &RedisRateLimiter{
		client:      client,
		maxAttempts: cfg.RateLimit.MaxAttempts,
		window:      cfg.RateLimit.WindowDuration,
	}
}

// Close closes the Redis connection
func (r *RedisRateLimiter) Close() error {
	return r.client.Close()
}

// LoginRateLimiter returns a Gin middleware that rate limits login attempts using Redis
func (r *RedisRateLimiter) LoginRateLimiter() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		key := fmt.Sprintf("login_attempts:%s", ip)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Increment attempt count
		val, err := r.client.Incr(ctx, key).Result()
		if err != nil {
			// If Redis fails, allow the request to proceed (fail open)
			// but log the error in production
			c.Next()
			return
		}

		// Set expiration on first request
		if val == 1 {
			r.client.Expire(ctx, key, r.window)
		}

		// Check if rate limit exceeded
		if int(val) > r.maxAttempts {
			remainingTime, _ := r.client.TTL(ctx, key).Result()
			response.Error(c, http.StatusTooManyRequests,
				fmt.Sprintf("Terlalu banyak percobaan login. Silakan coba lagi dalam %d Menit.", int(remainingTime.Seconds()/60)))
			c.Abort()
			return
		}

		c.Next()
	}
}

// ResetRateLimit resets the rate limit counter for an IP
func (r *RedisRateLimiter) ResetRateLimit(ctx context.Context, ip string) error {
	key := fmt.Sprintf("login_attempts:%s", ip)
	return r.client.Del(ctx, key).Err()
}
