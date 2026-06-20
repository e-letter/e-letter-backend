package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Refliqx/backend-eletter/internal/domain"
	"github.com/Refliqx/backend-eletter/internal/response"
	"github.com/Refliqx/backend-eletter/internal/service"
	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	svc service.AdminService
}

func NewAdminHandler(svc service.AdminService) *AdminHandler {
	return &AdminHandler{svc: svc}
}

func (h *AdminHandler) GetStats(c *gin.Context) {
	stats, err := h.svc.GetStats()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal memuat statistik")
		return
	}
	response.Success(c, http.StatusOK, "", stats)
}

func (h *AdminHandler) GetUsers(c *gin.Context) {
	role := c.Query("role")
	status := c.Query("status")
	search := c.Query("search")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))

	users, total, activeTotal, pendingTotal, err := h.svc.GetUsers(role, status, search, page, pageSize)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "", gin.H{"data": users, "total": total, "activeTotal": activeTotal, "pendingTotal": pendingTotal})
}

func (h *AdminHandler) UpdateUserStatus(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "ID tidak valid")
		return
	}
	var body struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.UpdateUserStatus(id, body.Status, adminUserID, c.ClientIP(), c.GetHeader("User-Agent")); err != nil {
		if err.Error() == "Status pengguna tidak valid" {
			response.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "Status berhasil diperbarui", nil)
}

func (h *AdminHandler) UpdateUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, http.StatusBadRequest, "ID tidak valid")
		return
	}
	var body struct {
		Role     *string `json:"role"`
		FullName *string `json:"full_name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	adminUserID := int64(toIntFromContext(c, "userId"))
	result, err := h.svc.UpdateUser(id, body.Role, body.FullName, adminUserID, c.ClientIP(), c.GetHeader("User-Agent"))
	if err != nil {
		if strings.Contains(err.Error(), "tidak ditemukan") {
			response.Error(c, http.StatusNotFound, err.Error())
			return
		}
		if strings.Contains(err.Error(), "Sudah ada kepala sekolah") {
			response.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "Pengguna berhasil diperbarui", result)
}

func (h *AdminHandler) CreateUser(c *gin.Context) {
	var body struct {
		FullName string `json:"full_name" binding:"required"`
		Email    string `json:"email" binding:"required,email"`
		Role     string `json:"role" binding:"required"`
		Status   string `json:"status"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	adminUserID := int64(toIntFromContext(c, "userId"))
	result, err := h.svc.CreateUser(body.FullName, body.Email, body.Role, body.Status, body.Password, adminUserID, c.ClientIP(), c.GetHeader("User-Agent"))
	if err != nil {
		if strings.Contains(err.Error(), "Sudah ada kepala sekolah") {
			response.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		if strings.Contains(err.Error(), "sudah terdaftar") {
			response.Error(c, http.StatusConflict, err.Error())
			return
		}
		if strings.Contains(err.Error(), "Peran") || strings.Contains(err.Error(), "Status") {
			response.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusCreated, "Pengguna berhasil dibuat", result)
}

func (h *AdminHandler) AdminDeleteLetter(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		response.Error(c, http.StatusBadRequest, "ID tidak valid")
		return
	}
	adminUserID := int64(toIntFromContext(c, "userId"))
	result, err := h.svc.AdminDeleteLetter(id, adminUserID, c.ClientIP(), c.GetHeader("User-Agent"))
	if err != nil {
		if err.Error() == "Surat tidak ditemukan atau sudah dihapus" {
			response.Error(c, http.StatusNotFound, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "Surat berhasil dihapus", result)
}

func (h *AdminHandler) GetRegistrationTokens(c *gin.Context) {
	tokens, err := h.svc.GetRegistrationTokens()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "", tokens)
}

func (h *AdminHandler) CreateRegistrationToken(c *gin.Context) {
	var body struct {
		Token      string  `json:"token" binding:"required"`
		RoleID     int     `json:"role_id" binding:"required"`
		UsageLimit int     `json:"usage_limit"`
		ExpiresAt  *string `json:"expires_at"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.CreateRegistrationToken(body.Token, body.RoleID, body.UsageLimit, body.ExpiresAt, adminUserID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusCreated, "Token berhasil dibuat", nil)
}

func (h *AdminHandler) DeleteRegistrationToken(c *gin.Context) {
	id := c.Param("id")
	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.DeleteRegistrationToken(id, adminUserID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "Token berhasil dihapus", nil)
}

func (h *AdminHandler) VerifyTeacherRole(c *gin.Context) {
	id := c.Param("id")
	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.VerifyTeacherRole(id, adminUserID, c.ClientIP(), c.GetHeader("User-Agent")); err != nil {
		if strings.Contains(err.Error(), "tidak ditemukan") || strings.Contains(err.Error(), "Invalid") {
			response.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "Peran guru berhasil diverifikasi dan penugasan dibuat", nil)
}

func (h *AdminHandler) ListPendingTeacherRoles(c *gin.Context) {
	statusVal := c.DefaultQuery("status", "pending")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	items, total, totalPages, currentPage, err := h.svc.ListPendingTeacherRoles(statusVal, page, limit)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal menghitung data: "+err.Error())
		return
	}
	response.Success(c, http.StatusOK, "", gin.H{"data": items, "meta": gin.H{
		"page": currentPage, "limit": limit, "total": total, "total_pages": totalPages,
	}})
}

func (h *AdminHandler) RejectTeacherRole(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body)
	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.RejectTeacherRole(id, adminUserID, c.ClientIP(), c.GetHeader("User-Agent")); err != nil {
		if strings.Contains(err.Error(), "Invalid") || strings.Contains(err.Error(), "bukan guru") || strings.Contains(err.Error(), "tidak ditemukan") {
			response.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "Permintaan peran ditolak", nil)
}

func (h *AdminHandler) GetAcademicYears(c *gin.Context) {
	items, err := h.svc.GetAcademicYears()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "", items)
}

func (h *AdminHandler) CreateAcademicYear(c *gin.Context) {
	var body struct {
		Name      string `json:"name" binding:"required"`
		StartDate string `json:"start_date" binding:"required"`
		EndDate   string `json:"end_date" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.CreateAcademicYear(body.Name, body.StartDate, body.EndDate, adminUserID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusCreated, "Tahun ajaran berhasil dibuat", nil)
}

func (h *AdminHandler) UpdateAcademicYear(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Name      *string `json:"name"`
		IsActive  *bool   `json:"is_active"`
		StartDate *string `json:"start_date"`
		EndDate   *string `json:"end_date"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.UpdateAcademicYear(id, body.Name, body.IsActive, body.StartDate, body.EndDate, adminUserID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "Tahun ajaran berhasil diperbarui", nil)
}

func (h *AdminHandler) DeleteAcademicYear(c *gin.Context) {
	id := c.Param("id")
	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.DeleteAcademicYear(id, adminUserID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "Tahun ajaran berhasil dihapus", nil)
}

func (h *AdminHandler) GetClasses(c *gin.Context) {
	items, err := h.svc.GetClasses()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "", items)
}

func (h *AdminHandler) CreateClass(c *gin.Context) {
	var body struct {
		ClassName      string `json:"class_name" binding:"required"`
		MajorID        int    `json:"major_id"`
		GradeLevel     int    `json:"grade_level"`
		AcademicYearID int    `json:"academic_year_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.GradeLevel == 0 {
		body.GradeLevel = 10
	}
	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.CreateClass(body.ClassName, body.MajorID, body.GradeLevel, body.AcademicYearID, adminUserID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusCreated, "Kelas berhasil dibuat", nil)
}

func (h *AdminHandler) UpdateClass(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		ClassName string `json:"class_name"`
		MajorID   int    `json:"major_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.UpdateClass(id, body.ClassName, body.MajorID, adminUserID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "Kelas berhasil diperbarui", nil)
}

func (h *AdminHandler) DeleteClass(c *gin.Context) {
	id := c.Param("id")
	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.DeleteClass(id, adminUserID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "Kelas berhasil dihapus", nil)
}

func (h *AdminHandler) GetMajors(c *gin.Context) {
	items, err := h.svc.GetMajors()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "", items)
}

func (h *AdminHandler) CreateMajor(c *gin.Context) {
	var body struct {
		MajorName  string `json:"major_name" binding:"required"`
		MajorShort string `json:"major_short"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.CreateMajor(body.MajorName, body.MajorShort, adminUserID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusCreated, "Konsentrasi Keahlian berhasil dibuat", nil)
}

func (h *AdminHandler) UpdateMajor(c *gin.Context) {
	var body struct {
		MajorName  string `json:"major_name"`
		MajorShort string `json:"major_short"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.UpdateMajor(c.Param("id"), body.MajorName, body.MajorShort, adminUserID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "Konsentrasi Keahlian berhasil diperbarui", nil)
}

func (h *AdminHandler) DeleteMajor(c *gin.Context) {
	id := c.Param("id")
	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.DeleteMajor(id, adminUserID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "Konsentrasi Keahlian berhasil dihapus", nil)
}

func (h *AdminHandler) GetSubjects(c *gin.Context) {
	items, err := h.svc.GetSubjects()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "", items)
}

func (h *AdminHandler) CreateSubject(c *gin.Context) {
	var body struct {
		SubjectName string `json:"subject_name" binding:"required"`
		SubjectCode string `json:"subject_code"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.CreateSubject(body.SubjectName, body.SubjectCode, adminUserID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusCreated, "Mata pelajaran berhasil dibuat", nil)
}

func (h *AdminHandler) UpdateSubject(c *gin.Context) {
	var body struct {
		SubjectName string `json:"subject_name"`
		SubjectCode string `json:"subject_code"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.UpdateSubject(c.Param("id"), body.SubjectName, body.SubjectCode, adminUserID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "Mata pelajaran berhasil diperbarui", nil)
}

func (h *AdminHandler) DeleteSubject(c *gin.Context) {
	id := c.Param("id")
	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.DeleteSubject(id, adminUserID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "Mata pelajaran berhasil dihapus", nil)
}

func (h *AdminHandler) GetSchedules(c *gin.Context) {
	items, err := h.svc.GetSchedules()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "", items)
}

func (h *AdminHandler) CreateSchedule(c *gin.Context) {
	var body struct {
		AcademicYearID int    `json:"academic_year_id" binding:"required"`
		ClassID        int    `json:"class_id" binding:"required"`
		SubjectID      int    `json:"subject_id" binding:"required"`
		TeacherID      int    `json:"teacher_id" binding:"required"`
		DayOfWeek      string `json:"day_of_week" binding:"required"`
		StartTime      string `json:"start_time" binding:"required"`
		EndTime        string `json:"end_time" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.CreateSchedule(body.AcademicYearID, body.ClassID, body.SubjectID, body.TeacherID, body.DayOfWeek, body.StartTime, body.EndTime, adminUserID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusCreated, "Jadwal berhasil dibuat", nil)
}

func (h *AdminHandler) UpdateSchedule(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		AcademicYearID int    `json:"academic_year_id"`
		ClassID        int    `json:"class_id"`
		SubjectID      int    `json:"subject_id"`
		TeacherID      int    `json:"teacher_id"`
		DayOfWeek      string `json:"day_of_week"`
		StartTime      string `json:"start_time"`
		EndTime        string `json:"end_time"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.UpdateSchedule(id, body.AcademicYearID, body.ClassID, body.SubjectID, body.TeacherID, body.DayOfWeek, body.StartTime, body.EndTime, adminUserID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "Jadwal berhasil diperbarui", nil)
}

func (h *AdminHandler) DeleteSchedule(c *gin.Context) {
	id := c.Param("id")
	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.DeleteSchedule(id, adminUserID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "Jadwal berhasil dihapus", nil)
}

func (h *AdminHandler) GetEnrollments(c *gin.Context) {
	classID := c.Query("class_id")
	search := strings.TrimSpace(c.Query("search"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	items, total, totalPages, currentPage, err := h.svc.GetEnrollments(classID, search, page, limit)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal menghitung data: "+err.Error())
		return
	}
	response.Success(c, http.StatusOK, "", gin.H{"data": items, "meta": gin.H{
		"page": currentPage, "limit": limit, "total": total, "total_pages": totalPages,
	}})
}

func (h *AdminHandler) CreateEnrollment(c *gin.Context) {
	var body struct {
		StudentID      int     `json:"student_id" binding:"required"`
		ClassID        int     `json:"class_id" binding:"required"`
		AcademicYearID int     `json:"academic_year_id"`
		Notes          *string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, "Data tidak valid. Periksa kembali input Anda.")
		return
	}
	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.CreateEnrollment(body.StudentID, body.ClassID, body.AcademicYearID, body.Notes, adminUserID, c.ClientIP(), c.GetHeader("User-Agent")); err != nil {
		if strings.Contains(err.Error(), "sudah terdaftar") {
			response.Error(c, http.StatusConflict, err.Error())
			return
		}
		if strings.Contains(err.Error(), "tidak ada tahun ajaran aktif") || strings.Contains(err.Error(), "tidak valid") {
			response.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusCreated, "Enrollment berhasil dibuat", nil)
}

func (h *AdminHandler) BulkPromoteStudents(c *gin.Context) {
	var body struct {
		Promotions []domain.BulkPromotionItem `json:"promotions" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, "Data kenaikan kelas tidak valid")
		return
	}

	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.BulkPromoteStudents(body.Promotions, adminUserID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "Kenaikan kelas berhasil diproses", nil)
}

func (h *AdminHandler) DeleteEnrollment(c *gin.Context) {
	id := c.Param("id")
	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.DeleteEnrollment(id, adminUserID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "Enrollment berhasil dihapus", nil)
}

func (h *AdminHandler) GetSchoolConfig(c *gin.Context) {
	config, err := h.svc.GetSchoolConfig()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "", config)
}

func (h *AdminHandler) GetPrincipalConfig(c *gin.Context) {
	fullName, signatureURL := h.svc.GetPrincipalConfig()
	response.Success(c, http.StatusOK, "", gin.H{"full_name": fullName, "signature_url": signatureURL})
}

func (h *AdminHandler) UpdateSchoolConfig(c *gin.Context) {
	var body map[string]string
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	adminUserID := int64(toIntFromContext(c, "userId"))
	if err := h.svc.UpdateSchoolConfig(body, adminUserID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "Konfigurasi berhasil diperbarui", nil)
}

func (h *AdminHandler) UploadConfigImage(c *gin.Context) {
	configKey := c.PostForm("config_key")
	if configKey == "" {
		response.Error(c, http.StatusBadRequest, "config_key diperlukan")
		return
	}

	allowedKeys := map[string]bool{
		"illustration_login_orange": true,
		"illustration_login_blue":   true,
		"illustration_register":     true,
		"bg_landing":                true,
		"app_logo":                  true,
		"school_logo":               true,
	}
	if !allowedKeys[configKey] {
		response.Error(c, http.StatusBadRequest, "config_key tidak valid")
		return
	}

	if strings.Contains(configKey, "/") || strings.Contains(configKey, "\\") || strings.Contains(configKey, "..") {
		response.Error(c, http.StatusBadRequest, "config_key tidak valid")
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "File diperlukan")
		return
	}

	if file.Size > 5*1024*1024 {
		response.Error(c, http.StatusBadRequest, "Ukuran file maksimal 5MB")
		return
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".svg" {
		response.Error(c, http.StatusBadRequest, "Format file tidak didukung (hanya PNG, JPG, JPEG, SVG)")
		return
	}

	contentType := file.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		response.Error(c, http.StatusBadRequest, "File harus berupa gambar")
		return
	}

	uploadDir := "public/uploads/config"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal membuat direktori upload")
		return
	}

	absUploadDir, err := filepath.Abs(uploadDir)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal memproses path")
		return
	}

	extensions := []string{".png", ".jpg", ".jpeg", ".svg"}
	for _, e := range extensions {
		if e != ext {
			candidatePath := filepath.Join(uploadDir, configKey+e)
			absCandidate, err := filepath.Abs(candidatePath)
			if err != nil || !strings.HasPrefix(absCandidate, absUploadDir) {
				continue
			}
			_ = os.Remove(candidatePath)
		}
	}

	filename := configKey + ext
	dst := filepath.Join(uploadDir, filename)

	absDst, err := filepath.Abs(dst)
	if err != nil || !strings.HasPrefix(absDst, absUploadDir) {
		response.Error(c, http.StatusBadRequest, "Path file tidak valid")
		return
	}

	if err := c.SaveUploadedFile(file, dst); err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal menyimpan file")
		return
	}

	filePath := "/uploads/config/" + filename
	adminUserID := int64(toIntFromContext(c, "userId"))
	if _, err := h.svc.UploadConfigImage(configKey, filePath, adminUserID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(c, http.StatusOK, "File berhasil diunggah", gin.H{
		"config_key": configKey,
		"file_path":  filePath,
	})
}

func (h *AdminHandler) GetAuditLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit < 1 {
		limit = 50
	}
	activityType := strings.TrimSpace(c.Query("activity_type"))
	search := strings.TrimSpace(c.Query("search"))

	logs, total, totalPages, currentPage, typeCounts, err := h.svc.GetAuditLogs(activityType, search, page, limit)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, http.StatusOK, "", gin.H{"data": logs, "meta": gin.H{
		"page": currentPage, "limit": limit, "total": total, "total_pages": totalPages,
	}, "type_counts": typeCounts})
}
