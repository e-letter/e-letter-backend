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
	Email     EmailConfig
}

type AdminConfig struct {
	Username string
	Password string
}

type KepsekConfig struct {
	Username string
	Password string
}

type EmailConfig struct {
	APIKey     string
	RedirectTo string
}

type AppConfig struct {
	Env            string
	Port           string
	BaseURL        string
	Timezone       string
	SchoolCode     string
	TrustedProxies []string
}

type DBConfig struct {
	Host            string
	Port            string
	User            string
	Password        string
	Name            string
	TLSEnabled      string
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
	LoginMax    int
	LoginWindow time.Duration

	LoginAdminMax    int
	LoginAdminWindow time.Duration

	RegisterMax    int
	RegisterWindow time.Duration

	ForgotPasswordMax    int
	ForgotPasswordWindow time.Duration
	VerifyOTPMax         int
	VerifyOTPWindow      time.Duration
	ResetPasswordMax     int
	ResetPasswordWindow  time.Duration

	RefreshMax    int
	RefreshWindow time.Duration

	WriteMax    int
	WriteWindow time.Duration

	ReadMax    int
	ReadWindow time.Duration

	SSEMax    int
	SSEWindow time.Duration

	AdminMax    int
	AdminWindow time.Duration

	DevMax    int
	DevWindow time.Duration

	GlobalMax    int
	GlobalWindow time.Duration
}
