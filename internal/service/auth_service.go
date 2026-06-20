package service

import "github.com/Refliqx/backend-eletter/internal/domain"

type AuthService interface {
	Register(req domain.RegisterRequest) (int, string, error)
	Login(req domain.LoginRequest, ip, userAgent string) (*domain.User, string, string, error)
	IssueAdminTokens(adminUsername string) (accessToken string, refreshToken string, userID int, err error)
	Refresh(refreshToken string) (string, string, error)
	Logout(refreshToken string) error
	VerifyAccessToken(accessToken string) (map[string]any, error)
	ForgotPassword(email, ip string) error
	VerifyOTP(email, otp string) error
	ResetPassword(email, otp, newPassword string) error
}
