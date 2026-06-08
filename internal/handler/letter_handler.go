package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Refliqx/backend-eletter/internal/domain"
	"github.com/Refliqx/backend-eletter/internal/response"
	"github.com/Refliqx/backend-eletter/internal/service"
	"github.com/Refliqx/backend-eletter/internal/utils"
	"github.com/gin-gonic/gin"
)

// isValidationError returns true if the error is a user-facing validation error (400) vs internal (500).
func isValidationError(err error) bool {
	msg := err.Error()
	return strings.HasPrefix(msg, "type_id,") ||
		strings.HasPrefix(msg, "pengajuan") ||
		strings.HasPrefix(msg, "format") ||
		strings.HasPrefix(msg, "jam")
}

type LetterHandler struct {
	service service.LetterService
	db      *sql.DB
}

func NewLetterHandler(s service.LetterService, db *sql.DB) *LetterHandler {
	return &LetterHandler{service: s, db: db}
}

// KepsekStats returns school-wide aggregate metrics for the principal dashboard.
// Queries are intentionally un-scoped (global) — no teacher_id / class_id filters.
func (h *LetterHandler) KepsekStats(c *gin.Context) {
	type stat struct {
		key, query string
	}
	queries := []stat{
		{"total_letters", `SELECT COUNT(*) FROM requests WHERE deleted_at IS NULL`},
		{"pending_kepsek", `SELECT COUNT(*) FROM request_approvals WHERE approver_role='kepala_sekolah' AND status='pending'`},
		{"approved_today", `SELECT COUNT(*) FROM request_approvals WHERE approver_role='kepala_sekolah' AND status='approved' AND DATE(acted_at)=CURDATE()`},
		{"total_students", `SELECT COUNT(*) FROM users WHERE role='student' AND deleted_at IS NULL`},
		{"total_teachers", `SELECT COUNT(*) FROM users WHERE role='teacher' AND deleted_at IS NULL`},
		{"approved_total", `SELECT COUNT(*) FROM requests WHERE status='approved' AND deleted_at IS NULL`},
	}
	stats := map[string]int{}
	for _, q := range queries {
		var n int
		_ = h.db.QueryRow(q.query).Scan(&n)
		stats[q.key] = n
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": stats})
}

// KepsekPending returns the letters that are currently pending the principal's approval.
//
// BUG FIX: The previous route mapped this to TeacherPending which used
// v_pending_approvals_for_teacher (joins teacher_profiles, approver_teacher_id).
// Kepsek approvals are stored with approver_principal_id, so those rows were
// NEVER returned. This handler queries request_approvals directly, resolving
// the current user's principal_profiles.id from the JWT userId.
func (h *LetterHandler) KepsekPending(c *gin.Context) {
	userID := toIntFromContext(c, "userId")
	if userID <= 0 {
		response.Raw(c, http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Resolve the principal profile id for this user.
	var principalID int
	err := h.db.QueryRow(
		`SELECT id FROM principal_profiles WHERE user_id = ? AND active = 1 AND deleted_at IS NULL LIMIT 1`,
		userID,
	).Scan(&principalID)
	if err != nil {
		// Not yet a principal profile — return empty, not an error.
		response.Raw(c, http.StatusOK, gin.H{
			"success": true,
			"data":    gin.H{"data": []any{}, "currentPage": 1, "totalPages": 0, "totalItems": 0},
		})
		return
	}

	page, limit := parsePagination(c)
	offset := (page - 1) * limit

	var totalItems int
	if err := h.db.QueryRow(`
		SELECT COUNT(*)
		FROM request_approvals ra
		JOIN requests r ON r.id = ra.request_id
		WHERE ra.approver_role = 'kepala_sekolah'
		  AND ra.approver_principal_id = ?
		  AND ra.status = 'pending'
		  AND r.deleted_at IS NULL
	`, principalID).Scan(&totalItems); err != nil {
		response.Raw(c, http.StatusInternalServerError, gin.H{"error": "Failed to count pending: " + err.Error()})
		return
	}

	rows, err := h.db.Query(`
		SELECT
			r.id,
			COALESCE(rt.label, 'Surat') AS type_label,
			COALESCE(r.reason, '')    AS reason,
			r.status,
			r.request_date,
			r.submitted_at,
			r.start_time,
			r.end_time,
			COALESCE(sp.full_name, tp.full_name, '')   AS requester_name,
			COALESCE(c.class_name, '-')                AS class_name,
			COALESCE(sp.student_code, tp.employee_code, '-') AS code,
			COALESCE(u.email, '-')                    AS email
		FROM request_approvals ra
		JOIN requests r  ON r.id  = ra.request_id
		JOIN request_types rt ON rt.id = r.request_type_id
		JOIN users u ON u.id = r.requester_user_id
		LEFT JOIN student_profiles sp ON sp.user_id = u.id
		LEFT JOIN teacher_profiles tp ON tp.user_id = u.id
		LEFT JOIN student_class_enrollments sce ON sce.student_id = sp.id AND sce.is_active = 1
		LEFT JOIN classes c ON c.id = sce.class_id
		WHERE ra.approver_role = 'kepala_sekolah'
		  AND ra.approver_principal_id = ?
		  AND ra.status = 'pending'
		  AND r.deleted_at IS NULL
		ORDER BY r.submitted_at DESC
		LIMIT ? OFFSET ?
	`, principalID, limit, offset)
	if err != nil {
		response.Raw(c, http.StatusInternalServerError, gin.H{"error": "Failed to fetch pending: " + err.Error()})
		return
	}
	defer rows.Close()

	type PendingItem struct {
		ID          int    `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Status      string `json:"status"`
		Date        string `json:"date"`
		SubmittedAt string `json:"submittedAt"`
		StartTime   string `json:"startTime"`
		EndTime     string `json:"endTime"`
		StudentName string `json:"studentName"`
		ClassName   string `json:"className"`
		Code        string `json:"code"`
		Email       string `json:"email"`
	}

	items := make([]PendingItem, 0)
	for rows.Next() {
		var (
			id                        int
			typeLabel, reason, status string
			requestDate, submittedAt  time.Time
			startTime, endTime        sql.NullString
			requesterName, className  string
			code, email               string
		)
		if err := rows.Scan(
			&id, &typeLabel, &reason, &status,
			&requestDate, &submittedAt, &startTime, &endTime,
			&requesterName, &className, &code, &email,
		); err != nil {
			response.Raw(c, http.StatusInternalServerError, gin.H{"error": "Scan error: " + err.Error()})
			return
		}
		start := "-"
		if startTime.Valid && len(startTime.String) >= 5 {
			start = startTime.String[:5]
		}
		end := "-"
		if endTime.Valid && len(endTime.String) >= 5 {
			end = endTime.String[:5]
		}
		items = append(items, PendingItem{
			ID:          id,
			Title:       typeLabel,
			Description: reason,
			Status:      "menunggu",
			Date:        requestDate.Format("2006-01-02"),
			SubmittedAt: submittedAt.Format("2006-01-02"),
			StartTime:   start,
			EndTime:     end,
			StudentName: requesterName,
			ClassName:   className,
			Code:        code,
			Email:       email,
		})
	}
	if err := rows.Err(); err != nil {
		response.Raw(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	totalPages := totalItems / limit
	if totalItems%limit != 0 {
		totalPages++
	}

	response.Raw(c, http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"data":        items,
			"currentPage": page,
			"totalPages":  totalPages,
			"totalItems":  totalItems,
		},
	})
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
		if isValidationError(err) {
			response.Raw(c, http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		response.Raw(c, http.StatusInternalServerError, gin.H{"success": false, "error": "Gagal membuat surat: " + err.Error()})
		return
	}
	utils.LogActivity(h.db, int64(userID), "create_letter", "Pembuatan surat baru ID #"+strconv.Itoa(id), c.ClientIP(), c.Request.UserAgent())
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
