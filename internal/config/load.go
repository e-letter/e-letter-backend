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
	maxLoginAttempts := mustParseIntWithFallback("RATE_LIMIT_MAX_ATTEMPTS", 5)
	rateLimitWindow := mustParseDurationWithFallback("RATE_LIMIT_WINDOW", 5*time.Minute)

	cfg := &Config{
		App: AppConfig{
			Env:            getEnv("APP_ENV", "development"),
			Port:           mustGetEnv("APP_PORT"),
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
			MaxAttempts:    maxLoginAttempts,
			WindowDuration: rateLimitWindow,
		},
		Admin: AdminConfig{
			Username: mustGetEnv("ADMIN_USERNAME"),
			Password: mustGetEnv("ADMIN_PASSWORD"),
		},
		Kepsek: KepsekConfig{
			Username: mustGetEnv("KEPSEK_USERNAME"),
			Password: mustGetEnv("KEPSEK_PASSWORD"),
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
