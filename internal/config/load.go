package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

func LoadConfig() *Config {
	env := os.Getenv("APP_ENV")
	if env == "" || env == "development" {
		if err := godotenv.Load(); err != nil {
			log.Println("No .env file found, falling back to system environment")
		}
	}

	dbMaxOpenConns := mustParseIntWithFallback("DB_MAX_OPEN_CONNS", 25)
	dbMaxIdleConns := mustParseIntWithFallback("DB_MAX_IDLE_CONNS", 10)
	dbConnMaxLifetime := mustParseDurationWithFallback("DB_CONN_MAX_LIFETIME", 30*time.Minute)
	dbConnMaxIdleTime := mustParseDurationWithFallback("DB_CONN_MAX_IDLE_TIME", 5*time.Minute)
	bcryptCost := mustParseIntWithFallback("BCRYPT_COST", 12)
	trustedProxies := parseTrustedProxies(getEnv("TRUSTED_PROXIES", "127.0.0.1"))
	redisDB := mustParseIntWithFallback("REDIS_DB", 0)

	cfg := &Config{
		App: AppConfig{
			Env:            getEnv("APP_ENV", "development"),
			Port:           mustGetEnv("APP_PORT"),
			BaseURL:        getEnv("APP_BASE_URL", "http://localhost:8080"),
			Timezone:       getEnv("APP_TIMEZONE", "Asia/Jakarta"),
			SchoolCode:     getEnv("SCHOOL_CODE", "SMKN2SGS"),
			TrustedProxies: trustedProxies,
		},
		DB: DBConfig{
			Host:            mustGetEnv("DB_HOST"),
			Port:            mustGetEnv("DB_PORT"),
			User:            mustGetEnv("DB_USER"),
			Password:        mustGetEnv("DB_PASSWORD"),
			Name:            mustGetEnv("DB_NAME"),
			TLSEnabled:      getEnv("DB_TLS", ""),
			MaxOpenConns:    dbMaxOpenConns,
			MaxIdleConns:    dbMaxIdleConns,
			ConnMaxLife:     dbConnMaxLifetime,
			ConnMaxIdleTime: dbConnMaxIdleTime,
		},
		JWT: JWTConfig{
			Secret:           mustGetEnv("JWT_SECRET"),
			AccessExpiresIn:  mustParseDurationWithFallback("JWT_EXPIRES_IN", 30*time.Minute),
			RefreshExpiresIn: mustParseDurationWithFallback("JWT_REFRESH_EXPIRES_IN", 30*24*time.Hour),
		},
		Security: SecurityConfig{
			BcryptCost: bcryptCost,
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnv("REDIS_PORT", "6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       redisDB,
		},
		RateLimit: RateLimitConfig{
			// Login (IP-based)
			LoginMax:         mustParseIntWithFallback("RATE_LIMIT_LOGIN_MAX", 5),
			LoginWindow:      mustParseDurationWithFallback("RATE_LIMIT_LOGIN_WINDOW", 5*time.Minute),
			LoginAdminMax:    mustParseIntWithFallback("RATE_LIMIT_LOGIN_ADMIN_MAX", 3),
			LoginAdminWindow: mustParseDurationWithFallback("RATE_LIMIT_LOGIN_ADMIN_WINDOW", 5*time.Minute),

			// Registration (IP-based)
			RegisterMax:    mustParseIntWithFallback("RATE_LIMIT_REGISTER_MAX", 3),
			RegisterWindow: mustParseDurationWithFallback("RATE_LIMIT_REGISTER_WINDOW", 10*time.Minute),

			// Password reset flow (IP-based)
			ForgotPasswordMax:    mustParseIntWithFallback("RATE_LIMIT_FORGOT_PASSWORD_MAX", 3),
			ForgotPasswordWindow: mustParseDurationWithFallback("RATE_LIMIT_FORGOT_PASSWORD_WINDOW", 15*time.Minute),
			VerifyOTPMax:         mustParseIntWithFallback("RATE_LIMIT_VERIFY_OTP_MAX", 5),
			VerifyOTPWindow:      mustParseDurationWithFallback("RATE_LIMIT_VERIFY_OTP_WINDOW", 10*time.Minute),
			ResetPasswordMax:     mustParseIntWithFallback("RATE_LIMIT_RESET_PASSWORD_MAX", 3),
			ResetPasswordWindow:  mustParseDurationWithFallback("RATE_LIMIT_RESET_PASSWORD_WINDOW", 15*time.Minute),

			// Token refresh (IP-based)
			RefreshMax:    mustParseIntWithFallback("RATE_LIMIT_REFRESH_MAX", 10),
			RefreshWindow: mustParseDurationWithFallback("RATE_LIMIT_REFRESH_WINDOW", 5*time.Minute),

			// Write operations (user-based)
			WriteMax:    mustParseIntWithFallback("RATE_LIMIT_WRITE_MAX", 10),
			WriteWindow: mustParseDurationWithFallback("RATE_LIMIT_WRITE_WINDOW", 1*time.Hour),

			// Read/list operations (user-based)
			ReadMax:    mustParseIntWithFallback("RATE_LIMIT_READ_MAX", 60),
			ReadWindow: mustParseDurationWithFallback("RATE_LIMIT_READ_WINDOW", 1*time.Minute),

			// SSE (user-based)
			SSEMax:    mustParseIntWithFallback("RATE_LIMIT_SSE_MAX", 1),
			SSEWindow: mustParseDurationWithFallback("RATE_LIMIT_SSE_WINDOW", 30*time.Second),

			// Admin panel (user-based)
			AdminMax:    mustParseIntWithFallback("RATE_LIMIT_ADMIN_MAX", 100),
			AdminWindow: mustParseDurationWithFallback("RATE_LIMIT_ADMIN_WINDOW", 1*time.Minute),

			// Dev endpoints (IP-based)
			DevMax:    mustParseIntWithFallback("RATE_LIMIT_DEV_MAX", 10),
			DevWindow: mustParseDurationWithFallback("RATE_LIMIT_DEV_WINDOW", 1*time.Minute),

			// Global fallback (IP-based)
			GlobalMax:    mustParseIntWithFallback("RATE_LIMIT_GLOBAL_MAX", 200),
			GlobalWindow: mustParseDurationWithFallback("RATE_LIMIT_GLOBAL_WINDOW", 1*time.Minute),
		},
		Admin: AdminConfig{
			Username: mustGetEnv("ADMIN_USERNAME"),
			Password: mustGetEnv("ADMIN_PASSWORD"),
		},
		Kepsek: KepsekConfig{
			Username: mustGetEnv("KEPSEK_USERNAME"),
			Password: mustGetEnv("KEPSEK_PASSWORD"),
		},
		Email: EmailConfig{
			APIKey:     getEnv("RESEND_API_KEY", ""),
			Sender:     getEnv("RESEND_FROM", getEnv("EMAIL_SENDER", "")),
			RedirectTo: getEnv("EMAIL_REDIRECT_TO", ""),
		},
	}

	return cfg
}

func getEnv(key, fallback string) string {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	return val
}

func mustGetEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("Environment variable %s is required", key)
	}
	return val
}

func mustParseIntWithFallback(key string, fallback int) int {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	intVal, err := strconv.Atoi(val)
	if err != nil {
		log.Printf("Invalid integer format for %s, using fallback %d", key, fallback)
		return fallback
	}
	return intVal
}

func mustParseDuration(key string) time.Duration {
	val := mustGetEnv(key)
	dur, err := time.ParseDuration(val)
	if err != nil {
		log.Fatalf("Invalid duration format for %s", key)
	}
	return dur
}

func mustParseDurationWithFallback(key string, fallback time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	dur, err := time.ParseDuration(val)
	if err != nil {
		log.Printf("Invalid duration format for %s, using fallback", key)
		return fallback
	}
	return dur
}

// parseTrustedProxies parses comma-separated list of trusted proxy IPs
func parseTrustedProxies(proxiesStr string) []string {
	if proxiesStr == "" {
		return []string{"127.0.0.1"}
	}
	var proxies []string
	for _, proxy := range strings.Split(proxiesStr, ",") {
		trimmed := strings.TrimSpace(proxy)
		if trimmed != "" {
			proxies = append(proxies, trimmed)
		}
	}
	if len(proxies) == 0 {
		return []string{"127.0.0.1"}
	}
	return proxies
}
