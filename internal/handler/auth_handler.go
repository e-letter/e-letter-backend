package handler

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Refliqx/backend-eletter/internal/config"
	"github.com/Refliqx/backend-eletter/internal/domain"
	"github.com/Refliqx/backend-eletter/internal/response"
	"github.com/Refliqx/backend-eletter/internal/service"
	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	service service.AuthService
	cfg     *config.Config
}

func NewAuthHandler(s service.AuthService, cfg *config.Config) *AuthHandler {
	return &AuthHandler{service: s, cfg: cfg}
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
		if strings.Contains(err.Error(), "terdaftar") {
			response.Error(c, http.StatusConflict, err.Error())
			return
		}
		response.Error(c, http.StatusBadRequest, err.Error())
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
		response.Error(c, http.StatusUnauthorized, "Kredensial admin tidak valid")
		return
	}

	accessToken, refreshToken, adminUserID, err := h.service.IssueAdminTokens(h.cfg.Admin.Username)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal membuat token")
		return
	}

	setRefreshCookie(c, refreshToken, 30*24*60*60)
	response.Raw(c, http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"user":        gin.H{"id": adminUserID, "email": "admin@system", "full_name": "Administrator", "role": "admin"},
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

	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Kata sandi berhasil diubah"})
}
