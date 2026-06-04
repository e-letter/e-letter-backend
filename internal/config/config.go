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

// EmailConfig holds SMTP credentials used for sending OTP emails.
type EmailConfig struct {
	Host     string // SMTP server host, e.g. smtp.gmail.com
	Port     string // SMTP server port, e.g. 587
	Sender   string // From address (EMAIL_SENDER)
	Password string // SMTP password or app-password (EMAIL_PASSWORD)
}

type AppConfig struct {
	Env            string
	Port           string
	BaseURL        string // Public base URL of the backend, e.g. https://api.example.com
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
	TLSEnabled      string // "true", "skip-verify", or empty (no TLS)
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
	// Login endpoints (IP-based) — prevents brute force per source
	LoginMax    int           // Max login attempts per IP
	LoginWindow time.Duration // Window for login rate limit

	// Admin/Kepsek login (IP-based, stricter)
	LoginAdminMax    int
	LoginAdminWindow time.Duration

	// Registration (IP-based)
	RegisterMax    int
	RegisterWindow time.Duration

	// Password reset flow (IP-based) — prevents OTP brute force & email flooding
	ForgotPasswordMax    int
	ForgotPasswordWindow time.Duration
	VerifyOTPMax         int
	VerifyOTPWindow      time.Duration
	ResetPasswordMax     int
	ResetPasswordWindow  time.Duration

	// Token refresh (IP-based, cookie-driven — no userId available)
	RefreshMax    int
	RefreshWindow time.Duration

	// Write operations (user-based) — each user gets own counter
	WriteMax    int
	WriteWindow time.Duration

	// Read/list operations (user-based)
	ReadMax    int
	ReadWindow time.Duration

	// SSE connections (user-based)
	SSEMax    int
	SSEWindow time.Duration

	// Admin panel operations (user-based)
	AdminMax    int
	AdminWindow time.Duration

	// Dev endpoints (IP-based)
	DevMax    int
	DevWindow time.Duration

	// Global fallback (IP-based) — DoS protection
	GlobalMax    int
	GlobalWindow time.Duration
}
