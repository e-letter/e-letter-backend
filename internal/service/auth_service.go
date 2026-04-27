package service

import "github.com/Refliqx/backend-eletter/internal/domain"

type AuthService interface {
	Register(req domain.RegisterRequest) (int, string, error)
	Login(req domain.LoginRequest, ip, userAgent string) (*domain.User, string, string, error)
	Refresh(refreshToken string) (string, string, error)
	Logout(refreshToken string) error
	VerifyAccessToken(accessToken string) (map[string]any, error)
}
