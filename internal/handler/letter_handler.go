package handler

import (
	"net/http"
	"strconv"

	"github.com/Refliqx/backend-eletter/internal/domain"
	"github.com/Refliqx/backend-eletter/internal/response"
	"github.com/Refliqx/backend-eletter/internal/service"
	"github.com/gin-gonic/gin"
)

type LetterHandler struct {
	service service.LetterService
}

func NewLetterHandler(s service.LetterService) *LetterHandler {
	return &LetterHandler{service: s}
}

func (h *LetterHandler) CreateStudent(c *gin.Context) { h.create(c) }
func (h *LetterHandler) CreateTeacher(c *gin.Context) { h.create(c) }

func (h *LetterHandler) create(c *gin.Context) {
	var req domain.LetterCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Raw(c, http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID := toIntFromContext(c, "userId")
	if userID <= 0 {
		response.Raw(c, http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	id, err := h.service.Create(userID, req)
	if err != nil {
		if err.Error() == "type_id, start_time, dan end_time diperlukan" {
			response.Raw(c, http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		response.Raw(c, http.StatusInternalServerError, gin.H{"success": false, "error": "Gagal membuat surat: " + err.Error()})
		return
	}
	response.Raw(c, http.StatusCreated, gin.H{"success": true, "data": gin.H{"request_id": id}})
}

func (h *LetterHandler) StudentIzinMasuk(c *gin.Context)  { h.listStudent(c, "izin_masuk") }
func (h *LetterHandler) StudentIzinKeluar(c *gin.Context) { h.listStudent(c, "izin_keluar") }
func (h *LetterHandler) StudentDispensasi(c *gin.Context) { h.listStudent(c, "dispensasi") }
func (h *LetterHandler) TeacherIzinMasuk(c *gin.Context)  { h.listTeacher(c, "izin_masuk") }
func (h *LetterHandler) TeacherIzinKeluar(c *gin.Context) { h.listTeacher(c, "izin_keluar") }
func (h *LetterHandler) TeacherDispensasi(c *gin.Context) { h.listTeacher(c, "dispensasi") }

func (h *LetterHandler) GeneralDispensasi(c *gin.Context) {
	userID := toIntFromContext(c, "userId")
	if userID <= 0 {
		response.Raw(c, http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	page, limit := parsePagination(c)
	resp, err := h.service.ListGeneralDispensasi("teacher", userID, page, limit)
	if err != nil {
		response.Raw(c, http.StatusInternalServerError, gin.H{"error": "Failed to fetch data: " + err.Error()})
		return
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": resp})
}

func (h *LetterHandler) TeacherLetters(c *gin.Context) {
	userID := toIntFromContext(c, "userId")
	if userID <= 0 {
		response.Raw(c, http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	page, limit := parsePagination(c)
	resp, err := h.service.ListTeacherLetters(userID, page, limit)
	if err != nil {
		response.Raw(c, http.StatusInternalServerError, gin.H{"error": "Failed to fetch data: " + err.Error()})
		return
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": resp})
}

func parsePagination(c *gin.Context) (int, int) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return page, limit
}

func (h *LetterHandler) listStudent(c *gin.Context, typeKey string) {
	userID := toIntFromContext(c, "userId")
	if userID <= 0 {
		response.Raw(c, http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	page, limit := parsePagination(c)
	resp, err := h.service.ListForStudent(userID, typeKey, page, limit)
	if err != nil {
		response.Raw(c, http.StatusInternalServerError, gin.H{"error": "Failed to fetch data: " + err.Error()})
		return
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": resp})
}

func (h *LetterHandler) listTeacher(c *gin.Context, typeKey string) {
	userID := toIntFromContext(c, "userId")
	if userID <= 0 {
		response.Raw(c, http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	page, limit := parsePagination(c)
	resp, err := h.service.ListForTeacherScoped(userID, typeKey, page, limit)
	if err != nil {
		if err.Error() == "forbidden: no active roles" {
			response.Raw(c, http.StatusForbidden, gin.H{"error": "Anda tidak memiliki peran aktif"})
			return
		}
		response.Raw(c, http.StatusInternalServerError, gin.H{"error": "Failed to fetch data: " + err.Error()})
		return
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": resp})
}

func (h *LetterHandler) TeacherPending(c *gin.Context) {
	userID := toIntFromContext(c, "userId")
	if userID <= 0 {
		response.Raw(c, http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	page, limit := parsePagination(c)
	resp, err := h.service.ListPendingForTeacher(userID, page, limit)
	if err != nil {
		if err.Error() == "forbidden: no active roles" {
			response.Raw(c, http.StatusForbidden, gin.H{"error": "Anda tidak memiliki peran aktif"})
			return
		}
		response.Raw(c, http.StatusInternalServerError, gin.H{"error": "Failed to fetch data: " + err.Error()})
		return
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": resp})
}

func (h *LetterHandler) TeacherStats(c *gin.Context) {
	userID := toIntFromContext(c, "userId")
	if userID <= 0 {
		response.Raw(c, http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	stats, err := h.service.GetTeacherStats(userID)
	if err != nil {
		response.Raw(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": stats})
}
