package handler

import (
	"net/http"
	"strconv"

	"github.com/Refliqx/backend-eletter/internal/response"
	"github.com/Refliqx/backend-eletter/internal/service"
	"github.com/gin-gonic/gin"
)

type NotificationHandler struct {
	service service.NotificationService
}

func NewNotificationHandler(s service.NotificationService) *NotificationHandler {
	return &NotificationHandler{service: s}
}

func (h *NotificationHandler) GetNotifications(c *gin.Context) {
	userIDVal, ok := c.Get("userId")
	if !ok {
		response.Error(c, http.StatusUnauthorized, "Token akses diperlukan")
		return
	}
	userID, _ := userIDVal.(int)

	notifications, err := h.service.GetNotifications(c.Request.Context(), int64(userID))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal mengambil notifikasi")
		return
	}

	response.Success(c, http.StatusOK, "Berhasil mengambil notifikasi", notifications)
}

func (h *NotificationHandler) MarkAsRead(c *gin.Context) {
	userIDVal, ok := c.Get("userId")
	if !ok {
		response.Error(c, http.StatusUnauthorized, "Token akses diperlukan")
		return
	}
	userID, _ := userIDVal.(int)

	notifID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "ID notifikasi tidak valid")
		return
	}

	if err := h.service.MarkAsRead(c.Request.Context(), notifID, int64(userID)); err != nil {
		response.Error(c, http.StatusNotFound, "Notifikasi tidak ditemukan")
		return
	}

	response.Success(c, http.StatusOK, "Notifikasi ditandai sudah dibaca", nil)
}
