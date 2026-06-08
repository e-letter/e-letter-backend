package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Refliqx/backend-eletter/internal/config"
	"github.com/Refliqx/backend-eletter/internal/response"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type KeyStrategy int

const (
	KeyByIP KeyStrategy = iota
	KeyByUserID
)

type RateLimitTier struct {
	Max    int
	Window time.Duration
	Key    KeyStrategy
}

type MultiRateLimiter struct {
	client *redis.Client
	config config.RateLimitConfig
}

func NewMultiRateLimiter(cfg *config.Config) *MultiRateLimiter {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", cfg.Redis.Host, cfg.Redis.Port),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	return &MultiRateLimiter{
		client: client,
		config: cfg.RateLimit,
	}
}

func (m *MultiRateLimiter) Close() error {
	return m.client.Close()
}

func (m *MultiRateLimiter) checkRateLimit(ctx context.Context, key string, max int, window time.Duration) (allowed bool, remaining int, retryAfter time.Duration, err error) {
	val, err := m.client.Incr(ctx, key).Result()
	if err != nil {
		return true, max, 0, err
	}

	ttl, err := m.client.TTL(ctx, key).Result()
	if err == nil && (val == 1 || ttl == -1*time.Nanosecond) {
		m.client.Expire(ctx, key, window)
	}

	remaining = max - int(val)
	if remaining < 0 {
		remaining = 0
	}

	if int(val) > max {
		ttl, _ := m.client.TTL(ctx, key).Result()
		if ttl > 0 {
			retryAfter = ttl
		} else {
			retryAfter = window
		}
		return false, 0, retryAfter, nil
	}

	return true, remaining, 0, nil
}

func buildKey(tierName string, identifier string) string {
	return fmt.Sprintf("rl:%s:%s", tierName, identifier)
}

func getIdentifier(c *gin.Context, strategy KeyStrategy) string {
	switch strategy {
	case KeyByIP:
		return c.ClientIP()
	case KeyByUserID:
		userId, exists := c.Get("userId")
		if !exists {
			return c.ClientIP()
		}
		switch v := userId.(type) {
		case int:
			return strconv.Itoa(v)
		case int64:
			return strconv.FormatInt(v, 10)
		case float64:
			return strconv.FormatFloat(v, 'f', 0, 64)
		case string:
			return v
		default:
			return fmt.Sprintf("%v", v)
		}
	default:
		return c.ClientIP()
	}
}

func setRateLimitHeaders(c *gin.Context, max int, remaining int, resetSeconds int) {
	c.Header("X-RateLimit-Limit", strconv.Itoa(max))
	c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
	c.Header("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Duration(resetSeconds)*time.Second).Unix(), 10))
}

func (m *MultiRateLimiter) genericMiddleware(tierName string, tier RateLimitTier) gin.HandlerFunc {
	return func(c *gin.Context) {
		identifier := getIdentifier(c, tier.Key)
		key := buildKey(tierName, identifier)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		allowed, remaining, retryAfter, err := m.checkRateLimit(ctx, key, tier.Max, tier.Window)
		if err != nil {
			c.Next()
			return
		}

		resetSeconds := int(tier.Window.Seconds())
		setRateLimitHeaders(c, tier.Max, remaining, resetSeconds)

		if !allowed {
			retryMinutes := int(retryAfter.Seconds() / 60)
			retrySecs := int(retryAfter.Seconds()) % 60
			var msg string
			if retryMinutes > 0 {
				msg = fmt.Sprintf("Terlalu banyak permintaan. Silakan coba lagi dalam %d menit %d detik.", retryMinutes, retrySecs)
			} else {
				msg = fmt.Sprintf("Terlalu banyak permintaan. Silakan coba lagi dalam %d detik.", retrySecs)
			}
			response.Error(c, http.StatusTooManyRequests, msg)
			c.Abort()
			return
		}

		c.Next()
	}
}

func (m *MultiRateLimiter) GlobalRateLimit() gin.HandlerFunc {
	return m.genericMiddleware("global", RateLimitTier{
		Max:    m.config.GlobalMax,
		Window: m.config.GlobalWindow,
		Key:    KeyByIP,
	})
}

func (m *MultiRateLimiter) LoginRateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("login", RateLimitTier{
		Max:    m.config.LoginMax,
		Window: m.config.LoginWindow,
		Key:    KeyByIP,
	})
}

func (m *MultiRateLimiter) LoginAdminRateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("login_admin", RateLimitTier{
		Max:    m.config.LoginAdminMax,
		Window: m.config.LoginAdminWindow,
		Key:    KeyByIP,
	})
}

func (m *MultiRateLimiter) RegisterRateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("register", RateLimitTier{
		Max:    m.config.RegisterMax,
		Window: m.config.RegisterWindow,
		Key:    KeyByIP,
	})
}

func (m *MultiRateLimiter) ForgotPasswordRateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("forgot_password", RateLimitTier{
		Max:    m.config.ForgotPasswordMax,
		Window: m.config.ForgotPasswordWindow,
		Key:    KeyByIP,
	})
}

func (m *MultiRateLimiter) VerifyOTPRateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("verify_otp", RateLimitTier{
		Max:    m.config.VerifyOTPMax,
		Window: m.config.VerifyOTPWindow,
		Key:    KeyByIP,
	})
}

func (m *MultiRateLimiter) ResetPasswordRateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("reset_password", RateLimitTier{
		Max:    m.config.ResetPasswordMax,
		Window: m.config.ResetPasswordWindow,
		Key:    KeyByIP,
	})
}

func (m *MultiRateLimiter) RefreshRateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("refresh", RateLimitTier{
		Max:    m.config.RefreshMax,
		Window: m.config.RefreshWindow,
		Key:    KeyByIP,
	})
}

func (m *MultiRateLimiter) WriteRateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("write", RateLimitTier{
		Max:    m.config.WriteMax,
		Window: m.config.WriteWindow,
		Key:    KeyByUserID,
	})
}

func (m *MultiRateLimiter) ReadRateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("read", RateLimitTier{
		Max:    m.config.ReadMax,
		Window: m.config.ReadWindow,
		Key:    KeyByUserID,
	})
}

func (m *MultiRateLimiter) SSERateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("sse", RateLimitTier{
		Max:    m.config.SSEMax,
		Window: m.config.SSEWindow,
		Key:    KeyByUserID,
	})
}

func (m *MultiRateLimiter) AdminRateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("admin", RateLimitTier{
		Max:    m.config.AdminMax,
		Window: m.config.AdminWindow,
		Key:    KeyByUserID,
	})
}

func (m *MultiRateLimiter) DevRateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("dev", RateLimitTier{
		Max:    m.config.DevMax,
		Window: m.config.DevWindow,
		Key:    KeyByIP,
	})
}

func (m *MultiRateLimiter) ResetRateLimit(ctx context.Context, ip string) error {
	tiers := []string{"login", "login_admin", "register", "forgot_password", "verify_otp", "reset_password", "refresh", "dev", "global"}
	pipe := m.client.Pipeline()
	for _, tier := range tiers {
		pipe.Del(ctx, buildKey(tier, ip))
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (m *MultiRateLimiter) ResetUserRateLimit(ctx context.Context, userID string) error {
	tiers := []string{"write", "read", "sse", "admin"}
	pipe := m.client.Pipeline()
	for _, tier := range tiers {
		pipe.Del(ctx, buildKey(tier, userID))
	}
	_, err := pipe.Exec(ctx)
	return err
}

var _ interface {
	ResetRateLimit(ctx context.Context, ip string) error
} = (*MultiRateLimiter)(nil)
