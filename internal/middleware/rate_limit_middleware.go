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

// KeyStrategy determines how the rate limit key is derived
type KeyStrategy int

const (
	KeyByIP     KeyStrategy = iota // Public endpoints — keyed by client IP
	KeyByUserID                    // Authenticated endpoints — keyed by user ID
)

// RateLimitTier defines a single rate limit configuration
type RateLimitTier struct {
	Max    int
	Window time.Duration
	Key    KeyStrategy
}

// MultiRateLimiter provides multi-tier Redis-based rate limiting
type MultiRateLimiter struct {
	client *redis.Client
	config config.RateLimitConfig
}

// NewMultiRateLimiter creates a new multi-tier rate limiter
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

// Close closes the Redis connection
func (m *MultiRateLimiter) Close() error {
	return m.client.Close()
}

// checkRateLimit is the core sliding-window counter implementation
func (m *MultiRateLimiter) checkRateLimit(ctx context.Context, key string, max int, window time.Duration) (allowed bool, remaining int, retryAfter time.Duration, err error) {
	val, err := m.client.Incr(ctx, key).Result()
	if err != nil {
		// Fail open — if Redis is unreachable, allow the request
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

// buildKey constructs the Redis key from a tier name and identifier
func buildKey(tierName string, identifier string) string {
	return fmt.Sprintf("rl:%s:%s", tierName, identifier)
}

// getIdentifier returns the correct identifier based on key strategy
func getIdentifier(c *gin.Context, strategy KeyStrategy) string {
	switch strategy {
	case KeyByIP:
		return c.ClientIP()
	case KeyByUserID:
		userId, exists := c.Get("userId")
		if !exists {
			// Fallback to IP if userId not in context (should not happen for user-based tiers)
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

// setRateLimitHeaders adds standard rate limit response headers
func setRateLimitHeaders(c *gin.Context, max int, remaining int, resetSeconds int) {
	c.Header("X-RateLimit-Limit", strconv.Itoa(max))
	c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
	c.Header("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Duration(resetSeconds)*time.Second).Unix(), 10))
}

// genericMiddleware creates a middleware for any tier
func (m *MultiRateLimiter) genericMiddleware(tierName string, tier RateLimitTier) gin.HandlerFunc {
	return func(c *gin.Context) {
		identifier := getIdentifier(c, tier.Key)
		key := buildKey(tierName, identifier)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		allowed, remaining, retryAfter, err := m.checkRateLimit(ctx, key, tier.Max, tier.Window)
		if err != nil {
			// Fail open
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

// ---------------------------------------------------------------------------
// Public middleware constructors — called from router.go
// ---------------------------------------------------------------------------

// GlobalRateLimit protects all API routes (IP-based, fallback DoS protection)
func (m *MultiRateLimiter) GlobalRateLimit() gin.HandlerFunc {
	return m.genericMiddleware("global", RateLimitTier{
		Max:    m.config.GlobalMax,
		Window: m.config.GlobalWindow,
		Key:    KeyByIP,
	})
}

// LoginRateLimiter protects student login (IP-based)
func (m *MultiRateLimiter) LoginRateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("login", RateLimitTier{
		Max:    m.config.LoginMax,
		Window: m.config.LoginWindow,
		Key:    KeyByIP,
	})
}

// LoginAdminRateLimiter protects admin/kepsek login (IP-based, stricter)
func (m *MultiRateLimiter) LoginAdminRateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("login_admin", RateLimitTier{
		Max:    m.config.LoginAdminMax,
		Window: m.config.LoginAdminWindow,
		Key:    KeyByIP,
	})
}

// RegisterRateLimiter protects registration (IP-based)
func (m *MultiRateLimiter) RegisterRateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("register", RateLimitTier{
		Max:    m.config.RegisterMax,
		Window: m.config.RegisterWindow,
		Key:    KeyByIP,
	})
}

// ForgotPasswordRateLimiter protects forgot-password (IP-based)
func (m *MultiRateLimiter) ForgotPasswordRateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("forgot_password", RateLimitTier{
		Max:    m.config.ForgotPasswordMax,
		Window: m.config.ForgotPasswordWindow,
		Key:    KeyByIP,
	})
}

// VerifyOTPRateLimiter protects OTP verification (IP-based)
func (m *MultiRateLimiter) VerifyOTPRateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("verify_otp", RateLimitTier{
		Max:    m.config.VerifyOTPMax,
		Window: m.config.VerifyOTPWindow,
		Key:    KeyByIP,
	})
}

// ResetPasswordRateLimiter protects password reset (IP-based)
func (m *MultiRateLimiter) ResetPasswordRateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("reset_password", RateLimitTier{
		Max:    m.config.ResetPasswordMax,
		Window: m.config.ResetPasswordWindow,
		Key:    KeyByIP,
	})
}

// RefreshRateLimiter protects token refresh (IP-based — cookie-driven, no userId)
func (m *MultiRateLimiter) RefreshRateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("refresh", RateLimitTier{
		Max:    m.config.RefreshMax,
		Window: m.config.RefreshWindow,
		Key:    KeyByIP,
	})
}

// WriteRateLimiter protects write operations (user-based — 1 user = 1 counter)
func (m *MultiRateLimiter) WriteRateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("write", RateLimitTier{
		Max:    m.config.WriteMax,
		Window: m.config.WriteWindow,
		Key:    KeyByUserID,
	})
}

// ReadRateLimiter protects read/list operations (user-based)
func (m *MultiRateLimiter) ReadRateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("read", RateLimitTier{
		Max:    m.config.ReadMax,
		Window: m.config.ReadWindow,
		Key:    KeyByUserID,
	})
}

// SSERateLimiter protects SSE connections (user-based)
func (m *MultiRateLimiter) SSERateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("sse", RateLimitTier{
		Max:    m.config.SSEMax,
		Window: m.config.SSEWindow,
		Key:    KeyByUserID,
	})
}

// AdminRateLimiter protects admin panel operations (user-based)
func (m *MultiRateLimiter) AdminRateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("admin", RateLimitTier{
		Max:    m.config.AdminMax,
		Window: m.config.AdminWindow,
		Key:    KeyByUserID,
	})
}

// DevRateLimiter protects dev endpoints (IP-based)
func (m *MultiRateLimiter) DevRateLimiter() gin.HandlerFunc {
	return m.genericMiddleware("dev", RateLimitTier{
		Max:    m.config.DevMax,
		Window: m.config.DevWindow,
		Key:    KeyByIP,
	})
}

// ResetRateLimit resets all rate limit keys for a given IP (called on successful login)
func (m *MultiRateLimiter) ResetRateLimit(ctx context.Context, ip string) error {
	// Collect all IP-based tier names
	tiers := []string{"login", "login_admin", "register", "forgot_password", "verify_otp", "reset_password", "refresh", "dev", "global"}
	pipe := m.client.Pipeline()
	for _, tier := range tiers {
		pipe.Del(ctx, buildKey(tier, ip))
	}
	_, err := pipe.Exec(ctx)
	return err
}

// ResetUserRateLimit resets all rate limit keys for a given user ID
func (m *MultiRateLimiter) ResetUserRateLimit(ctx context.Context, userID string) error {
	tiers := []string{"write", "read", "sse", "admin"}
	pipe := m.client.Pipeline()
	for _, tier := range tiers {
		pipe.Del(ctx, buildKey(tier, userID))
	}
	_, err := pipe.Exec(ctx)
	return err
}

// Interface compliance: ensure AuthHandler.RateLimiter interface is satisfied
var _ interface {
	ResetRateLimit(ctx context.Context, ip string) error
} = (*MultiRateLimiter)(nil)
