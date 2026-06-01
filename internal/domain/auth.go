package domain

import "time"

type LoginRequest struct {
	ID         string `json:"id"`
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

// GetLoginID returns the effective login identifier, preferring "identifier" over "id"
func (r *LoginRequest) GetLoginID() string {
	if r.Identifier != "" {
		return r.Identifier
	}
	return r.ID
}

type RegisterRequest struct {
	FullName string `json:"fullName"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type TokenRecord struct {
	TokenID    int
	UserID     int
	TokenHash  string
	TokenType  string
	IsRevoked  bool
	ExpiresAt  *time.Time
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

type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required"`
}

type VerifyOTPRequest struct {
	Email string `json:"email" binding:"required"`
	OTP   string `json:"otp" binding:"required"`
}

type ResetPasswordRequest struct {
	Email       string `json:"email" binding:"required"`
	OTP         string `json:"otp" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

type AuthRepository interface {
	GetUserByLoginIdentifiers(id string) (*User, error)
	GetUserByEmail(email string) (*User, error)
	// GetUserByEmailAnyStatus returns a user regardless of their status (active/pending/inactive).
	// Used during registration to prevent duplicate email registrations even for pending accounts.
	GetUserByEmailAnyStatus(email string) (*User, error)
	CreateUser(roleID int, email, passwordHash, status string) (int, error)
	UpdateUserProfile(userID int, fullName string, profileCompleted bool) error
	GetUserByID(userID int) (*User, error)
	GetRegistrationToken(token string) (*TokenRecord, error)
	IncrementRegistrationTokenUsage(token string) error
	StoreRefreshToken(userID int, tokenHash string, expiresAt time.Time) error
	GetRefreshToken(tokenHash string) (*TokenRecord, error)
	RevokeRefreshToken(tokenHash string) error
	RevokeRefreshTokensByUserID(userID int) error
	LogLoginAttempt(attempt LoginAttempt) error
	GetTeacherSubRoles(userID int) []string
	CreatePasswordResetToken(userID int, otpHash string, expiresAt time.Time, ip string) error
	VerifyPasswordResetOTP(email, otpHash string) (int, error)
	MarkPasswordResetUsed(email, otpHash string) error
	UpdatePassword(userID int, passwordHash string) error
}

type AuthService interface {
	Register(req RegisterRequest) (int, string, error)
	Login(req LoginRequest, ip, userAgent string) (*User, string, string, error)
	Refresh(refreshToken string) (string, string, error)
	Logout(refreshToken string) error
	VerifyAccessToken(accessToken string) (map[string]any, error)
	ForgotPassword(email, ip string) error
	VerifyOTP(email, otp string) error
	ResetPassword(email, otp, newPassword string) error
}
