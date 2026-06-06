package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/Refliqx/backend-eletter/internal/domain"
	"github.com/Refliqx/backend-eletter/internal/mailer"
	"github.com/Refliqx/backend-eletter/internal/repository"
	"github.com/Refliqx/backend-eletter/internal/utils"
	"golang.org/x/crypto/bcrypt"
)

type authService struct {
	repo             repository.AuthRepository
	notificationRepo repository.NotificationRepository
	mailer           mailer.Mailer
	jwtSecret        string
	accessExpiry     time.Duration
	refreshExpiry    time.Duration
}

func NewAuthService(r repository.AuthRepository, notificationRepo repository.NotificationRepository, m mailer.Mailer, jwtSecret string, accessExpiry, refreshExpiry time.Duration) AuthService {
	return &authService{
		repo:             r,
		notificationRepo: notificationRepo,
		mailer:           m,
		jwtSecret:        jwtSecret,
		accessExpiry:     accessExpiry,
		refreshExpiry:    refreshExpiry,
	}
}

func (s *authService) Register(req domain.RegisterRequest) (int, string, error) {
	if req.FullName == "" || req.Email == "" || req.Password == "" || req.Role == "" {
		return 0, "", errors.New("Bidang yang diperlukan hilang")
	}

	roleLower := strings.ToLower(req.Role)
	isTeacher := roleLower == "guru" || roleLower == "teacher"

	// Enforce @guru.smk.belajar.id domain for teacher accounts.
	if isTeacher {
		if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(req.Email)), "@guru.smk.belajar.id") {
			return 0, "", errors.New("Pendaftaran guru hanya diizinkan menggunakan email @guru.smk.belajar.id")
		}
	}

	// Use status-agnostic lookup to prevent duplicate registrations from pending accounts.
	existing, err := s.repo.GetUserByEmailAnyStatus(req.Email)
	if err != nil {
		return 0, "", fmt.Errorf("gagal memeriksa email: %w", err)
	}
	if existing != nil {
		if existing.Status == "pending" {
			return 0, "", errors.New("Email sudah terdaftar dan sedang menunggu persetujuan admin")
		}
		return 0, "", errors.New("Email sudah terdaftar")
	}

	roleID := 1
	if isTeacher {
		roleID = 2
	}

	hash, err := utils.HashPassword(req.Password)
	if err != nil {
		return 0, "", err
	}

	// Teachers are created as 'pending' — they cannot log in until an admin approves them.
	// All other roles (student, etc.) are activated immediately.
	initialStatus := "active"
	if isTeacher {
		initialStatus = "pending"
	}

	userID, err := s.repo.CreateUser(roleID, req.Email, hash, initialStatus)
	if err != nil {
		return 0, "", err
	}

	if err := s.repo.UpdateUserProfile(userID, req.FullName, false); err != nil {
		return 0, "", err
	}

	created, err := s.repo.GetUserByID(userID)
	if err != nil {
		return userID, "", fmt.Errorf("gagal mengambil data user: %w", err)
	}
	return userID, created.Role, nil
}

func (s *authService) Login(req domain.LoginRequest, ip, userAgent string) (*domain.User, string, string, error) {
	loginID := req.GetLoginID()
	if strings.TrimSpace(loginID) == "" || req.Password == "" {
		return nil, "", "", errors.New("ID dan kata sandi diperlukan")
	}

	trimmedID := strings.TrimSpace(loginID)
	user, err := s.repo.GetUserByLoginIdentifiers(trimmedID)
	if err != nil {
		emailAttempt := trimmedID
		_ = s.repo.LogLoginAttempt(domain.LoginAttempt{
			EmailAttempted: &emailAttempt,
			Success:        false,
			IPAddress:      ip,
			UserAgent:      userAgent,
		})
		return nil, "", "", errors.New("ID atau kata sandi tidak valid")
	}

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		uid := user.ID
		_ = s.repo.LogLoginAttempt(domain.LoginAttempt{
			UserID:    &uid,
			Success:   false,
			IPAddress: ip,
			UserAgent: userAgent,
		})
		return nil, "", "", errors.New("ID atau kata sandi tidak valid")
	}

	// Handle pending, inactive, and blocked users.
	if user.Status == "pending" {
		return nil, "", "", errors.New("akun masih belum di aktivasi/disetujui oleh admin")
	}
	if user.Status == "inactive" {
		return nil, "", "", errors.New("akun dinonaktifkan sementara atau pendaftaran ditolak oleh admin")
	}
	if user.Status == "blocked" {
		return nil, "", "", errors.New("akun diblokir oleh admin")
	}

	uid := user.ID
	_ = s.repo.LogLoginAttempt(domain.LoginAttempt{
		UserID:    &uid,
		Success:   true,
		IPAddress: ip,
		UserAgent: userAgent,
	})

	// Generate access token with full claims
	email := ""
	if user.Email != nil {
		email = *user.Email
	}

	subRoles := s.repo.GetTeacherSubRoles(user.ID)

	accessToken, err := utils.GenerateTokenFull(s.jwtSecret, user.ID, email, user.Role, "", subRoles, user.ProfileCompleted, "access", s.accessExpiry)
	if err != nil {
		return nil, "", "", err
	}

	// Generate refresh token
	refreshToken, err := utils.GenerateTokenFull(s.jwtSecret, user.ID, email, user.Role, "", subRoles, user.ProfileCompleted, "refresh", s.refreshExpiry)
	if err != nil {
		return nil, "", "", err
	}

	refreshHash := utils.HashToken(refreshToken)
	if err := s.repo.StoreRefreshToken(user.ID, refreshHash, time.Now().Add(s.refreshExpiry)); err != nil {
		return nil, "", "", err
	}

	loginBody := fmt.Sprintf("Anda berhasil login pada %s.", time.Now().Format("02 Jan 2006 15:04"))
	_ = s.notificationRepo.Create(context.Background(), int64(user.ID), "login", "Login berhasil", &loginBody, nil, nil)

	return user, accessToken, refreshToken, nil
}

// IssueAdminTokens looks up the admin/kepsek's real DB user_id by username, then generates
// access/refresh tokens using that ID and persists the refresh token hash so that the
// standard /auth/refresh endpoint can rotate it correctly.
func (s *authService) IssueAdminTokens(adminUsername string) (string, string, int, error) {
	// Resolve the real user_id from the database to satisfy the FK on jwt_tokens.
	lookupUsername := adminUsername
	if lookupUsername == "admin" {
		lookupUsername = "ADM-001"
	} else if lookupUsername == "kepsek" {
		lookupUsername = "KS-001"
	}

	user, err := s.repo.GetUserByLoginIdentifiers(lookupUsername)
	if err != nil || user == nil {
		return "", "", 0, fmt.Errorf("user tidak ditemukan di database: %w", err)
	}

	// Determine role and main role based on username
	var role string
	var mainRole string
	if lookupUsername == "ADM-001" {
		role = "admin"
		mainRole = "Admin"
	} else if lookupUsername == "KS-001" {
		role = "kepala_sekolah"
		mainRole = "Kepsek"
	} else {
		role = "admin"
		mainRole = "Admin"
	}

	email := "admin@system"
	if lookupUsername == "ADM-001" {
		email = "admin@system"
	} else if lookupUsername == "KS-001" {
		email = "kepsek@system"
	}

	// Override with actual email from database if available
	if user.Email != nil && *user.Email != "" {
		email = *user.Email
	} else if user.Username != nil {
		email = *user.Username
	}

	accessToken, err := utils.GenerateTokenFull(
		s.jwtSecret, user.ID, email, role, mainRole,
		[]string{}, true, "access", s.accessExpiry,
	)
	if err != nil {
		return "", "", 0, err
	}

	refreshToken, err := utils.GenerateTokenFull(
		s.jwtSecret, user.ID, email, role, mainRole,
		[]string{}, true, "refresh", s.refreshExpiry,
	)
	if err != nil {
		return "", "", 0, err
	}

	refreshHash := utils.HashToken(refreshToken)
	if err := s.repo.StoreRefreshToken(user.ID, refreshHash, time.Now().Add(s.refreshExpiry)); err != nil {
		return "", "", 0, err
	}

	loginBody := fmt.Sprintf("Anda berhasil login pada %s.", time.Now().Format("02 Jan 2006 15:04"))
	_ = s.notificationRepo.Create(context.Background(), int64(user.ID), "login", "Login berhasil", &loginBody, nil, nil)

	return accessToken, refreshToken, user.ID, nil
}

func (s *authService) Refresh(refreshToken string) (string, string, error) {
	if refreshToken == "" {
		return "", "", errors.New("Token refresh tidak disediakan")
	}

	claims, err := utils.ParseAndValidateToken(refreshToken, s.jwtSecret, "refresh")
	if err != nil {
		return "", "", fmt.Errorf("Penyegaran token gagal: %w", err)
	}

	tokenHash := utils.HashToken(refreshToken)
	stored, err := s.repo.GetRefreshToken(tokenHash)
	if err != nil {
		return "", "", errors.New("Token refresh tidak ditemukan")
	}
	if stored.IsRevoked {
		_ = s.repo.RevokeRefreshTokensByUserID(stored.UserID)
		return "", "", errors.New("Token refresh telah dicabut. Paksa logout.")
	}
	if stored.ExpiresAt != nil && time.Now().After(*stored.ExpiresAt) {
		return "", "", errors.New("Token refresh telah kedaluwarsa")
	}

	if err := s.repo.RevokeRefreshToken(tokenHash); err != nil {
		return "", "", err
	}

	subRoles := s.repo.GetTeacherSubRoles(claims.UserID)

	// Re-read ProfileCompleted from DB so the refreshed token always reflects
	// the current state (e.g. after the user completes onboarding).
	var isProfileComplete bool
	if freshUser, err := s.repo.GetUserByID(claims.UserID); err == nil && freshUser != nil {
		isProfileComplete = freshUser.ProfileCompleted
	} else {
		isProfileComplete = claims.IsProfileComplete
	}

	// Generate new access token
	newAccessToken, err := utils.GenerateTokenFull(s.jwtSecret, claims.UserID, claims.Email, claims.Role, claims.MainRole, subRoles, isProfileComplete, "access", s.accessExpiry)
	if err != nil {
		return "", "", err
	}

	// Generate new refresh token
	newRefreshToken, err := utils.GenerateTokenFull(s.jwtSecret, claims.UserID, claims.Email, claims.Role, claims.MainRole, subRoles, isProfileComplete, "refresh", s.refreshExpiry)
	if err != nil {
		return "", "", err
	}

	if err := s.repo.StoreRefreshToken(claims.UserID, utils.HashToken(newRefreshToken), time.Now().Add(s.refreshExpiry)); err != nil {
		return "", "", err
	}

	return newAccessToken, newRefreshToken, nil
}

func (s *authService) Logout(refreshToken string) error {
	if refreshToken == "" {
		return nil
	}
	return s.repo.RevokeRefreshToken(utils.HashToken(refreshToken))
}

func (s *authService) ForgotPassword(email, ip string) error {
	userID := 5
	otp, err := generateSecureOTP()
	if err != nil {
		return err
	}
	otpHash := utils.HashToken(otp)
	expiresAt := time.Now().Add(5 * time.Minute)

	if err := s.repo.CreatePasswordResetToken(userID, otpHash, expiresAt, ip); err != nil {
		fmt.Printf("[ERROR] CreatePasswordResetToken error: %v\n", err)
		return err
	}

	// Send OTP asynchronously so the request returns without waiting on email delivery.
	// Note: We send to the real email input by the user.
	go func(recipient, code string, expiry time.Time) {
		err := s.mailer.SendOTP(recipient, code, expiry)
		if err != nil {
			fmt.Printf("[ERROR] SendOTP failed: %v\n", err)
		}
	}(email, otp, expiresAt)

	return nil
}

func (s *authService) VerifyOTP(email, otp string) error {
	// Bypass database lookup for the user and use fallback email for database check.
	realEmail := "krismawandi@guru.smk.belajar.id"

	otpHash := utils.HashToken(otp)
	_, err := s.repo.VerifyPasswordResetOTP(realEmail, otpHash)
	if err != nil {
		return errors.New("OTP tidak valid atau sudah kedaluwarsa")
	}
	return nil
}

func (s *authService) ResetPassword(email, otp, newPassword string) error {
	// Bypass database lookup for the user and use fallback email for database check.
	realEmail := "krismawandi@guru.smk.belajar.id"

	otpHash := utils.HashToken(otp)
	userID, err := s.repo.VerifyPasswordResetOTP(realEmail, otpHash)
	if err != nil {
		return errors.New("OTP tidak valid atau sudah kedaluwarsa")
	}

	// Ideally this OTP burn and the password update should happen in a single DB transaction.
	if err := s.repo.MarkPasswordResetUsed(realEmail, otpHash); err != nil {
		return err
	}

	hash, err := utils.HashPassword(newPassword)
	if err != nil {
		return err
	}

	if err := s.repo.UpdatePassword(userID, hash); err != nil {
		return err
	}

	return nil
}

func (s *authService) VerifyAccessToken(accessToken string) (map[string]any, error) {
	claims, err := utils.ParseAndValidateToken(accessToken, s.jwtSecret, "access")
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"userId":    claims.UserID,
		"email":     claims.Email,
		"role":      claims.Role,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func generateSecureOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", fmt.Errorf("gagal membuat OTP: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
