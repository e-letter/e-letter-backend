package handler

import (
	"net/http"
	"strings"

	"github.com/Refliqx/backend-eletter/internal/domain"
	"github.com/Refliqx/backend-eletter/internal/response"
	"github.com/Refliqx/backend-eletter/internal/service"
	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	service service.AuthService
}

func NewAuthHandler(s service.AuthService) *AuthHandler {
	return &AuthHandler{s}
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
		"data": gin.H{
			"id":         userID,
			"login_code": loginCode,
			"user_code":  loginCode,
		},
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

	c.SetCookie("refreshToken", refreshToken, 30*24*60*60, "/", "", false, true)
	response.Raw(c, http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"user": gin.H{
				"id":         user.ID,
				"email":      user.Email,
				"full_name":  user.FullName,
				"role":       user.RoleID,
				"login_code": user.LoginCode,
			},
			"accessToken": accessToken,
		},
	})
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	refreshToken, err := c.Cookie("refreshToken")
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Token refresh tidak disediakan")
		return
	}

	accessToken, newRefreshToken, svcErr := h.service.Refresh(refreshToken)
	if svcErr != nil {
		c.SetCookie("refreshToken", "", -1, "/", "", false, true)
		response.Error(c, http.StatusUnauthorized, svcErr.Error())
		return
	}

	c.SetCookie("refreshToken", newRefreshToken, 30*24*60*60, "/", "", false, true)
	response.Raw(c, http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"accessToken": accessToken,
		},
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	refreshToken, _ := c.Cookie("refreshToken")
	_ = h.service.Logout(refreshToken)
	c.SetCookie("refreshToken", "", -1, "/", "", false, true)
	response.Raw(c, http.StatusOK, gin.H{
		"success": true,
		"message": "Logout berhasil",
	})
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
		response.Error(c, http.StatusInternalServerError, "Gagal mengakses sumber daya terlindungi: "+err.Error())
		return
	}

	response.Raw(c, http.StatusOK, gin.H{
		"success": true,
		"message": "Akses diberikan ke sumber daya terlindungi",
		"data":    data,
	})
}
