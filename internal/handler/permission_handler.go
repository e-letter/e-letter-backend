package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/Refliqx/backend-eletter/internal/domain"
	"github.com/Refliqx/backend-eletter/internal/response"
	"github.com/Refliqx/backend-eletter/internal/service"
	"github.com/Refliqx/backend-eletter/internal/utils"
	"github.com/gin-gonic/gin"
)

type PermissionHandler struct {
	service service.PermissionService
	isDev   bool
	db      *sql.DB
}

func NewPermissionHandler(s service.PermissionService, isDev bool, db *sql.DB) *PermissionHandler {
	return &PermissionHandler{service: s, isDev: isDev, db: db}
}

func (h *PermissionHandler) GetRequests(c *gin.Context) {
	idSiswa := c.Query("id_siswa")
	action := c.Query("action")
	nisn := c.Query("nisn")
	startDate := c.Query("startDate")
	endDate := c.Query("endDate")
	search := c.Query("search")
	status := c.Query("status")
	typeKey := c.Query("type_key")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}

	userID := toIntFromContext(c, "userId")
	userRole := c.GetString("userRole")
	roleID := 0
	if userRole == "admin" {
		roleID = 2
	} else if userRole == "teacher" {
		roleID = 1
	}

	out, err := h.service.Get(action, idSiswa, nisn, userID, roleID, startDate, endDate, search, status, typeKey, page, limit)
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
	userID := toIntFromContext(c, "userId")
	utils.LogActivity(h.db, int64(userID), "create_request", "Pengajuan permohonan baru ID #"+strconv.Itoa(id), c.ClientIP(), c.Request.UserAgent())
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
	userID := toIntFromContext(c, "userId")
	if err := h.service.Approve(req, userID); err != nil {
		if req.RequestID == 0 || req.StageID == 0 || req.Status == "" || err.Error() == "invalid status: must be approved, rejected, or skipped" {
			response.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, "Failed to process approval: "+err.Error())
		return
	}
	utils.LogActivity(h.db, int64(userID), "approve_"+req.Status, "Permohonan #"+strconv.Itoa(req.RequestID)+" status: "+req.Status, c.ClientIP(), c.Request.UserAgent())
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

func (h *PermissionHandler) CancelRequest(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)
	if id == 0 {
		response.Error(c, http.StatusBadRequest, "ID permintaan diperlukan")
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body)

	userID := toIntFromContext(c, "userId")
	if err := h.service.CancelRequest(id, userID, body.Reason); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.LogActivity(h.db, int64(userID), "cancel_request", "Pembatalan permohonan #"+strconv.Itoa(id), c.ClientIP(), c.Request.UserAgent())
	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Permintaan berhasil dibatalkan"})
}

func (h *PermissionHandler) GetRequestDetail(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)
	if id == 0 {
		response.Error(c, http.StatusBadRequest, "ID permintaan diperlukan")
		return
	}

	detail, err := h.service.GetRequestDetail(id)
	if err != nil {
		response.Error(c, http.StatusNotFound, "Permintaan tidak ditemukan")
		return
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": detail})
}

func (h *PermissionHandler) GetTeacherRoles(c *gin.Context) {
	userID := toIntFromContext(c, "userId")
	roles, err := h.service.GetTeacherRoles(userID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": roles})
}

func (h *PermissionHandler) RequestTeacherRole(c *gin.Context) {
	userID := toIntFromContext(c, "userId")
	var body struct {
		RoleName        string `json:"role_name" binding:"required"`
		HomeroomClassID *int   `json:"homeroom_class_id"`
		MajorID         *int   `json:"major_id"`
		SubjectIDs      []int  `json:"subject_ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	meta := domain.TeacherRoleMetadata{
		HomeroomClassID: body.HomeroomClassID,
		MajorID:         body.MajorID,
		SubjectIDs:      body.SubjectIDs,
	}
	if err := h.service.RequestTeacherRole(userID, body.RoleName, meta); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.LogActivity(h.db, int64(userID), "request_teacher_role", "Permintaan peran guru: "+body.RoleName, c.ClientIP(), c.Request.UserAgent())
	response.Raw(c, http.StatusCreated, gin.H{"success": true, "message": "Permintaan peran berhasil diajukan"})
}

func (h *PermissionHandler) CreateDelegation(c *gin.Context) {
	userID := toIntFromContext(c, "userId")
	var body struct {
		DelegateUserID int    `json:"delegate_user_id" binding:"required"`
		ValidFrom      string `json:"valid_from" binding:"required"`
		ValidUntil     string `json:"valid_until" binding:"required"`
		Reason         string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.CreateDelegation(userID, body.DelegateUserID, body.ValidFrom, body.ValidUntil, body.Reason); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.LogActivity(h.db, int64(userID), "create_delegation", "Delegasi ke user ID #"+strconv.Itoa(body.DelegateUserID), c.ClientIP(), c.Request.UserAgent())
	response.Raw(c, http.StatusCreated, gin.H{"success": true, "message": "Delegasi berhasil dibuat"})
}

func (h *PermissionHandler) ListDelegations(c *gin.Context) {
	userID := toIntFromContext(c, "userId")
	delegations, err := h.service.ListDelegations(userID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": delegations})
}

func (h *PermissionHandler) DeleteDelegation(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)
	userID := toIntFromContext(c, "userId")
	if err := h.service.DeleteDelegation(id, userID); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.LogActivity(h.db, int64(userID), "delete_delegation", "Hapus delegasi ID #"+strconv.Itoa(id), c.ClientIP(), c.Request.UserAgent())
	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Delegasi berhasil dihapus"})
}
