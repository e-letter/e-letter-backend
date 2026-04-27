package domain

import "time"

type LoginRequest struct {
	ID       string `json:"id"`
	Password string `json:"password"`
}

type RegisterRequest struct {
	FullName string `json:"fullName"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
	Token    string `json:"token"`
}

type TokenRecord struct {
	TokenID    int
	UserID     int
	TokenHash  string
	TokenType  string
	IsRevoked  bool
	ExpiresAt  time.Time
	UsageLimit *int
	UsedCount  int
}

type LoginAttempt struct {
	UserID         *int
	EmailAttempted *string
	Success        bool
	IPAddress      string
	UserAgent      string
}

type AuthRepository interface {
	GetUserByLoginIdentifiers(id string) (*User, error)
	GetUserByEmail(email string) (*User, error)
	CreateUser(roleID int, email, passwordHash string) (int, error)
	UpdateUserProfile(userID int, fullName string, profileCompleted bool) error
	GetUserByID(userID int) (*User, error)
	GetRegistrationToken(token string) (*TokenRecord, error)
	IncrementRegistrationTokenUsage(token string) error
	StoreRefreshToken(userID int, tokenHash string, expiresAt time.Time) error
	GetRefreshToken(tokenHash string) (*TokenRecord, error)
	RevokeRefreshToken(tokenHash string) error
	RevokeRefreshTokensByUserID(userID int) error
	LogLoginAttempt(attempt LoginAttempt) error
}

type AuthService interface {
	Register(req RegisterRequest) (int, string, error)
	Login(req LoginRequest, ip, userAgent string) (*User, string, string, error)
	Refresh(refreshToken string) (string, string, error)
	Logout(refreshToken string) error
	VerifyAccessToken(accessToken string) (map[string]any, error)
}
