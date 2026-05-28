package config

import "time"

type Config struct {
	App       AppConfig
	DB        DBConfig
	JWT       JWTConfig
	Security  SecurityConfig
	Redis     RedisConfig
	RateLimit RateLimitConfig
	Admin     AdminConfig
	Kepsek    KepsekConfig
}

type AdminConfig struct {
	Username string
	Password string
}

type KepsekConfig struct {
	Username string
	Password string
}

type AppConfig struct {
	Env            string
	Port           string
	Timezone       string
	SchoolCode     string
	TrustedProxies []string // List of trusted proxy IPs
}

type DBConfig struct {
	Host            string
	Port            string
	User            string
	Password        string
	Name            string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLife     time.Duration
	ConnMaxIdleTime time.Duration
}

type JWTConfig struct {
	Secret           string
	AccessExpiresIn  time.Duration
	RefreshExpiresIn time.Duration
}

type SecurityConfig struct {
	BcryptCost int
}

type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

type RateLimitConfig struct {
	MaxAttempts    int           // Maximum login attempts
	WindowDuration time.Duration // Time window for rate limiting
}
