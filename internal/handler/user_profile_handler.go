package handler

import (
	"net/http"

	"github.com/Refliqx/backend-eletter/internal/domain"
	"github.com/Refliqx/backend-eletter/internal/response"
	"github.com/Refliqx/backend-eletter/internal/service"
	"github.com/gin-gonic/gin"
)

type UserProfileHandler struct {
	service service.UserProfileService
}

func NewUserProfileHandler(s service.UserProfileService) *UserProfileHandler {
	return &UserProfileHandler{service: s}
}

func (h *UserProfileHandler) GetProfile(c *gin.Context) {
	userIDVal, ok := c.Get("userId")
	if !ok {
		response.Error(c, http.StatusUnauthorized, "Token akses diperlukan")
		return
	}
	userID, _ := userIDVal.(int)
	user, err := h.service.GetProfile(userID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal mengambil data profil: "+err.Error())
		return
	}
	if user == nil {
		response.Error(c, http.StatusNotFound, "User tidak ditemukan")
		return
	}
	response.Raw(c, http.StatusOK, gin.H{
		"success": true,
		"data":    user,
	})
}

func (h *UserProfileHandler) UpdateProfile(c *gin.Context) {
	var req domain.UserProfileUpdatePayload
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	user, err := h.service.UpdateProfile(req)
	if err != nil {
		if err.Error() == "Missing userId" {
			response.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "user": user})
}
