package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Refliqx/backend-eletter/internal/domain"
	"github.com/Refliqx/backend-eletter/internal/response"
	"github.com/Refliqx/backend-eletter/internal/service"
	"github.com/gin-gonic/gin"
)

type PermissionHandler struct {
	service service.PermissionService
	isDev   bool
}

func NewPermissionHandler(s service.PermissionService, isDev bool) *PermissionHandler {
	return &PermissionHandler{service: s, isDev: isDev}
}

func (h *PermissionHandler) GetRequests(c *gin.Context) {
	idSiswa := c.Query("id_siswa")
	action := c.Query("action")
	nisn := c.Query("nisn")

	userID, _ := c.Get("userId")
	roleID, _ := c.Get("userRole")

	out, err := h.service.Get(action, idSiswa, nisn, toInt(userID), toInt(roleID))
	if err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "Token akses diperlukan" {
			status = http.StatusUnauthorized
		}
		if err.Error() == "User not found" || err.Error() == "User tidak ditemukan" {
			status = http.StatusNotFound
		}
		response.Error(c, status, err.Error())
		return
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": out})
}

func (h *PermissionHandler) CreateRequest(c *gin.Context) {
	var req domain.CreatePermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	id, err := h.service.Create(req)
	if err != nil {
		if err.Error() == "Missing required fields" {
			response.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, "Failed to create request: "+err.Error())
		return
	}
	response.Raw(c, http.StatusCreated, gin.H{"success": true, "data": gin.H{"request_id": id}})
}

func (h *PermissionHandler) UpdateRequest(c *gin.Context) {
	var req domain.UpdatePermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.Update(req); err != nil {
		if err.Error() == "request_id is required" {
			response.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, "Failed to update request: "+err.Error())
		return
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Request updated successfully"})
}

func (h *PermissionHandler) DeleteRequest(c *gin.Context) {
	idStr := c.Query("id")
	id, _ := strconv.Atoi(idStr)
	if err := h.service.Delete(id); err != nil {
		if err.Error() == "id is required" {
			response.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, "Failed to delete request: "+err.Error())
		return
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Request deleted successfully"})
}

func (h *PermissionHandler) Approve(c *gin.Context) {
	var req domain.ApprovalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	userID, _ := c.Get("userId")
	if err := h.service.Approve(req, toInt(userID)); err != nil {
		if req.RequestID == 0 || req.StageID == 0 || req.Status == "" || err.Error() == "Invalid status. Must be APPROVED, REJECTED, or FORWARDED" {
			response.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, "Failed to process approval: "+err.Error())
		return
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Request processed successfully"})
}

func (h *PermissionHandler) ListRegistrationTokens(c *gin.Context) {
	if !h.isDev {
		response.Error(c, http.StatusForbidden, "Not allowed in production")
		return
	}
	rows, err := h.service.ListRegistrationTokens()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": rows})
}

func (h *PermissionHandler) UpsertRegistrationToken(c *gin.Context) {
	if !h.isDev {
		response.Error(c, http.StatusForbidden, "Not allowed in production")
		return
	}
	var body struct {
		Token      string  `json:"token"`
		RoleID     int     `json:"role_id"`
		UsageLimit *int    `json:"usage_limit"`
		ExpiresAt  *string `json:"expires_at"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	var expiresAt *time.Time
	if body.ExpiresAt != nil && *body.ExpiresAt != "" {
		parsed, err := time.Parse(time.RFC3339, *body.ExpiresAt)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "invalid expires_at format")
			return
		}
		expiresAt = &parsed
	}
	row, err := h.service.UpsertRegistrationToken(body.Token, body.RoleID, body.UsageLimit, expiresAt)
	if err != nil {
		if err.Error() == "Missing token or role_id" {
			response.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": row})
}

func toInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	default:
		return 0
	}
}
