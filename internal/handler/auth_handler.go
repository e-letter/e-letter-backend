package handler

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Refliqx/backend-eletter/internal/config"
	"github.com/Refliqx/backend-eletter/internal/domain"
	"github.com/Refliqx/backend-eletter/internal/response"
	"github.com/Refliqx/backend-eletter/internal/service"
	"github.com/Refliqx/backend-eletter/internal/utils"
	"github.com/gin-gonic/gin"
)

type RateLimiter interface {
	ResetRateLimit(ctx context.Context, ip string) error
}

type AuthHandler struct {
	service     service.AuthService
	cfg         *config.Config
	rateLimiter RateLimiter
	db          *sql.DB
}

func NewAuthHandler(s service.AuthService, cfg *config.Config, rl RateLimiter, db *sql.DB) *AuthHandler {
	return &AuthHandler{service: s, cfg: cfg, rateLimiter: rl, db: db}
}

func setRefreshCookie(c *gin.Context, value string, maxAge int) {
	secure := os.Getenv("APP_ENV") == "production"
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("refreshToken", value, maxAge, "/", "", secure, true)
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req domain.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	userID, loginCode, err := h.service.Register(req)
	if err != nil {
		// Only map the two specific duplicate-email errors to 409 Conflict.
		// Avoid matching DB trigger messages like "tidak terdaftar di ref_values"
		// which contain the same substring but are unrelated validation errors.
		errMsg := err.Error()
		if errMsg == "Email sudah terdaftar" || errMsg == "Email sudah terdaftar dan sedang menunggu persetujuan admin" {
			response.Error(c, http.StatusConflict, errMsg)
			return
		}
		response.Error(c, http.StatusBadRequest, errMsg)
		return
	}

	response.Raw(c, http.StatusCreated, gin.H{
		"success": true,
		"data":    gin.H{"id": userID, "login_code": loginCode, "user_code": loginCode},
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req domain.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	user, accessToken, refreshToken, err := h.service.Login(req, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		if strings.Contains(err.Error(), "diperlukan") {
			response.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		response.Error(c, http.StatusUnauthorized, err.Error())
		return
	}

	if h.rateLimiter != nil {
		_ = h.rateLimiter.ResetRateLimit(c.Request.Context(), c.ClientIP())
	}

	setRefreshCookie(c, refreshToken, 30*24*60*60)
	response.Raw(c, http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"user":        gin.H{"id": user.ID, "email": user.Email, "full_name": user.FullName, "role": user.Role, "login_code": user.Username},
			"accessToken": accessToken,
		},
	})
}

func (h *AuthHandler) AdminLogin(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	if req.Username != h.cfg.Admin.Username || req.Password != h.cfg.Admin.Password {
		utils.LogActivity(h.db, 0, "login_failed", fmt.Sprintf("Login admin gagal untuk username: %s", req.Username), c.ClientIP(), c.GetHeader("User-Agent"))
		response.Error(c, http.StatusUnauthorized, "Kredensial admin tidak valid")
		return
	}

	if h.rateLimiter != nil {
		_ = h.rateLimiter.ResetRateLimit(c.Request.Context(), c.ClientIP())
	}

	accessToken, refreshToken, adminUserID, err := h.service.IssueAdminTokens(h.cfg.Admin.Username)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal membuat token")
		return
	}

	setRefreshCookie(c, refreshToken, 30*24*60*60)
	utils.LogActivity(h.db, int64(adminUserID), "login", "Admin login berhasil", c.ClientIP(), c.GetHeader("User-Agent"))
	response.Raw(c, http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"user":        gin.H{"id": adminUserID, "email": "admin@system", "full_name": "Administrator", "role": "admin"},
			"accessToken": accessToken,
		},
	})
}

func (h *AuthHandler) KepsekLogin(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	if req.Username != h.cfg.Kepsek.Username || req.Password != h.cfg.Kepsek.Password {
		utils.LogActivity(h.db, 0, "login_failed", fmt.Sprintf("Login kepsek gagal untuk username: %s", req.Username), c.ClientIP(), c.GetHeader("User-Agent"))
		response.Error(c, http.StatusUnauthorized, "Kredensial kepsek tidak valid")
		return
	}

	if h.rateLimiter != nil {
		_ = h.rateLimiter.ResetRateLimit(c.Request.Context(), c.ClientIP())
	}

	accessToken, refreshToken, kepsekUserID, err := h.service.IssueAdminTokens(h.cfg.Kepsek.Username)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal membuat token")
		return
	}

	setRefreshCookie(c, refreshToken, 30*24*60*60)
	utils.LogActivity(h.db, int64(kepsekUserID), "login", "Kepala Sekolah login berhasil", c.ClientIP(), c.GetHeader("User-Agent"))
	response.Raw(c, http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"user":        gin.H{"id": kepsekUserID, "email": "kepsek@system", "full_name": "Kepala Sekolah", "role": "kepala_sekolah"},
			"accessToken": accessToken,
		},
	})
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	refreshToken, err := c.Cookie("refreshToken")
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Token refresh tidak disediakan. Force logout")
		return
	}

	accessToken, newRefreshToken, svcErr := h.service.Refresh(refreshToken)
	if svcErr != nil {
		setRefreshCookie(c, "", -1)
		response.Error(c, http.StatusUnauthorized, svcErr.Error()+" Force logout")
		return
	}

	setRefreshCookie(c, newRefreshToken, 30*24*60*60)
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": gin.H{"accessToken": accessToken}})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	refreshToken, _ := c.Cookie("refreshToken")
	_ = h.service.Logout(refreshToken)
	setRefreshCookie(c, "", -1)
	userID := toIntFromContext(c, "userId")
	utils.LogActivity(h.db, int64(userID), "logout", "User logout", c.ClientIP(), c.GetHeader("User-Agent"))
	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Logout berhasil"})
}

func (h *AuthHandler) Protected(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if token == "" {
		response.Error(c, http.StatusUnauthorized, "Token akses diperlukan")
		return
	}

	data, err := h.service.VerifyAccessToken(token)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Gagal mengakses sumber daya terlindungi: "+err.Error())
		return
	}

	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Akses diberikan ke sumber daya terlindungi", "data": data})
}

func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req domain.ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Email diperlukan")
		return
	}

	_ = h.service.ForgotPassword(req.Email, c.ClientIP())
	utils.LogActivity(h.db, 0, "forgot_password", fmt.Sprintf("Permintaan reset password untuk email: %s", req.Email), c.ClientIP(), c.GetHeader("User-Agent"))
	// Always return success to not reveal if email exists
	response.Raw(c, http.StatusOK, gin.H{
		"success": true,
		"message": "Jika email terdaftar, kode OTP telah dikirim",
		"data":    gin.H{"expires_in": 15 * time.Minute / time.Second},
	})
}

func (h *AuthHandler) VerifyOTP(c *gin.Context) {
	var req domain.VerifyOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Email dan OTP diperlukan")
		return
	}

	if err := h.service.VerifyOTP(req.Email, req.OTP); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "OTP valid"})
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req domain.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Email, OTP, dan kata sandi baru diperlukan")
		return
	}

	if len(req.NewPassword) < 6 {
		response.Error(c, http.StatusBadRequest, "Kata sandi minimal 6 karakter")
		return
	}

	if err := h.service.ResetPassword(req.Email, req.OTP, req.NewPassword); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.LogActivity(h.db, 0, "reset_password", fmt.Sprintf("Reset password berhasil untuk email: %s", req.Email), c.ClientIP(), c.GetHeader("User-Agent"))
	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Kata sandi berhasil diubah"})
}
