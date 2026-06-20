package handler

import (
	"database/sql"
	"fmt"
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

func isValidationError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.HasPrefix(msg, "type_id") ||
		strings.HasPrefix(msg, "pengajuan") ||
		strings.HasPrefix(msg, "format") ||
		strings.HasPrefix(msg, "jam") ||
		strings.HasPrefix(msg, "start_time diperlukan") ||
		strings.HasPrefix(msg, "izin keluar/masuk") ||
		strings.HasPrefix(msg, "dispensasi hanya") ||
		strings.HasPrefix(msg, "sudah ada surat izin aktif") ||
		strings.HasPrefix(msg, "user pengaju tidak ditemukan") ||
		strings.HasPrefix(msg, "jenis surat tidak ditemukan") ||
		strings.HasPrefix(msg, "jenis surat tidak aktif") ||
		strings.HasPrefix(msg, "tanggal selesai harus setelah atau sama dengan tanggal mulai")
}

type LetterHandler struct {
	service service.LetterService
	db      *sql.DB
}

func NewLetterHandler(s service.LetterService, db *sql.DB) *LetterHandler {
	return &LetterHandler{service: s, db: db}
}

type monthlyTrendItem struct {
	Month      string `json:"month"`
	IzinMasuk  int    `json:"izin_masuk"`
	IzinKeluar int    `json:"izin_keluar"`
	Dispen     int    `json:"dispensasi"`
}

type compositionItem struct {
	Label string `json:"label"`
	Code  string `json:"code"`
	Count int    `json:"count"`
	Color string `json:"color"`
}

type hourlyApprovalItem struct {
	Hour  int `json:"hour"`
	Count int `json:"count"`
}

type classDistItem struct {
	ClassName string `json:"class_name"`
	Count     int    `json:"count"`
}

type topStudentItem struct {
	Name  string `json:"name"`
	Class string `json:"class"`
	Count int    `json:"count"`
}

type violationItem struct {
	Name           string `json:"name"`
	ClassName      string `json:"class_name"`
	ViolationType  string `json:"violation_type"`
	Frequency      string `json:"frequency"`
	Recommendation string `json:"recommendation"`
}

var monthNames = map[int]string{
	1: "Jan", 2: "Feb", 3: "Mar", 4: "Apr",
	5: "Mei", 6: "Jun", 7: "Jul", 8: "Agu",
	9: "Sep", 10: "Okt", 11: "Nov", 12: "Des",
}

var compositionColors = map[string]string{
	"izin_masuk":  "#3b82f6",
	"izin_keluar": "#ef4444",
	"dispensasi":  "#8b5cf6",
}

func parseYearPeriod(c *gin.Context) (year int, monthStart, monthEnd int) {
	year = time.Now().Year()
	period := c.DefaultQuery("period", "Tahunan")

	if y, err := strconv.Atoi(c.DefaultQuery("year", "")); err == nil && y > 0 {
		year = y
	}

	switch period {
	case "Semester 1":
		monthStart, monthEnd = 1, 6
	case "Semester 2":
		monthStart, monthEnd = 7, 12
	default:
		monthStart, monthEnd = 1, 12
	}
	return
}

func dateWhere(year, monthStart, monthEnd int, tableAlias string) string {
	alias := tableAlias
	if alias != "" {
		alias += "."
	}
	return fmt.Sprintf(`%srequest_date IS NOT NULL AND YEAR(%srequest_date) = %d AND MONTH(%srequest_date) BETWEEN %d AND %d`,
		alias, alias, year, alias, monthStart, monthEnd)
}

func (h *LetterHandler) KepsekStats(c *gin.Context) {
	year, monthStart, monthEnd := parseYearPeriod(c)

	queries := []struct {
		key, query string
	}{
		{"total_letters", fmt.Sprintf(`SELECT COUNT(*) FROM requests WHERE deleted_at IS NULL AND %s`, dateWhere(year, monthStart, monthEnd, ""))},
		{"pending_kepsek", `SELECT COUNT(*) FROM request_approvals WHERE approver_role='kepala_sekolah' AND status='pending'`},
		{"approved_today", `SELECT COUNT(*) FROM request_approvals WHERE approver_role='kepala_sekolah' AND status='approved' AND DATE(acted_at)=CURDATE()`},
		{"total_students", `SELECT COUNT(*) FROM users WHERE role='student' AND deleted_at IS NULL`},
		{"total_teachers", `SELECT COUNT(*) FROM users WHERE role='teacher' AND deleted_at IS NULL`},
		{"approved_total", fmt.Sprintf(`SELECT COUNT(*) FROM requests WHERE status='approved' AND deleted_at IS NULL AND %s`, dateWhere(year, monthStart, monthEnd, ""))},
	}
	stats := map[string]int{}
	for _, q := range queries {
		var n int
		_ = h.db.QueryRow(q.query).Scan(&n)
		stats[q.key] = n
	}

	monthlyTrend := buildMonthlyTrend(h, year, monthStart, monthEnd)
	composition := buildComposition(h, year, monthStart, monthEnd)
	hourlyApprovals := buildHourlyApprovals(h, year, monthStart, monthEnd)
	classDist := buildClassDistribution(h, year, monthStart, monthEnd)
	topStud := buildTopStudents(h, year, monthStart, monthEnd)
	slaAvg, slaPerf := buildSLAMetrics(h, year, monthStart, monthEnd)
	violations := buildViolations(h, year, monthStart, monthEnd)

	response.Success(c, http.StatusOK, "", gin.H{
		"total_letters":      stats["total_letters"],
		"pending_kepsek":     stats["pending_kepsek"],
		"approved_today":     stats["approved_today"],
		"total_students":     stats["total_students"],
		"total_teachers":     stats["total_teachers"],
		"approved_total":     stats["approved_total"],
		"monthly_trend":      monthlyTrend,
		"composition":        composition,
		"hourly_approvals":   hourlyApprovals,
		"class_distribution": classDist,
		"top_students":       topStud,
		"sla_avg_minutes":    slaAvg,
		"sla_performance":    slaPerf,
		"violations":         violations,
	})
}

func buildMonthlyTrend(h *LetterHandler, year, monthStart, monthEnd int) []monthlyTrendItem {
	query := fmt.Sprintf(`
		SELECT MONTH(r.request_date) as month_num, rt.code, COUNT(*) as cnt
		FROM requests r
		JOIN request_types rt ON r.request_type_id = rt.id
		WHERE r.deleted_at IS NULL AND r.status != 'draft'
		  AND %s
		GROUP BY MONTH(r.request_date), rt.code
		ORDER BY month_num, rt.code
	`, dateWhere(year, monthStart, monthEnd, "r"))
	rows, err := h.db.Query(query)
	if err != nil {
		return []monthlyTrendItem{}
	}
	defer rows.Close()

	type rawMonth struct {
		num  int
		code string
		cnt  int
	}
	var rawData []rawMonth
	for rows.Next() {
		var m int
		var code string
		var cnt int
		if err := rows.Scan(&m, &code, &cnt); err != nil {
			continue
		}
		rawData = append(rawData, rawMonth{m, code, cnt})
	}

	monthMap := make(map[int]*monthlyTrendItem)
	for _, d := range rawData {
		if _, ok := monthMap[d.num]; !ok {
			monthMap[d.num] = &monthlyTrendItem{
				Month: monthNames[d.num],
			}
		}
		switch d.code {
		case "izin_masuk":
			monthMap[d.num].IzinMasuk = d.cnt
		case "izin_keluar":
			monthMap[d.num].IzinKeluar = d.cnt
		case "dispensasi":
			monthMap[d.num].Dispen = d.cnt
		}
	}

	result := make([]monthlyTrendItem, 0, len(monthMap))
	for i := monthStart; i <= monthEnd; i++ {
		if m, ok := monthMap[i]; ok {
			result = append(result, *m)
		}
	}
	return result
}

func buildComposition(h *LetterHandler, year, monthStart, monthEnd int) []compositionItem {
	query := fmt.Sprintf(`
		SELECT rt.label, rt.code, COUNT(*) as cnt
		FROM requests r
		JOIN request_types rt ON r.request_type_id = rt.id
		WHERE r.deleted_at IS NULL AND r.status != 'draft'
		  AND %s
		GROUP BY rt.id, rt.label, rt.code
		ORDER BY rt.id
	`, dateWhere(year, monthStart, monthEnd, "r"))
	rows, err := h.db.Query(query)
	if err != nil {
		return []compositionItem{}
	}
	defer rows.Close()

	var result []compositionItem
	for rows.Next() {
		var label, code string
		var cnt int
		if err := rows.Scan(&label, &code, &cnt); err != nil {
			continue
		}
		color := compositionColors[code]
		if color == "" {
			color = "#6b7280"
		}
		result = append(result, compositionItem{
			Label: label,
			Code:  code,
			Count: cnt,
			Color: color,
		})
	}
	return result
}

func buildHourlyApprovals(h *LetterHandler, year, monthStart, monthEnd int) []hourlyApprovalItem {
	query := fmt.Sprintf(`
		SELECT HOUR(ra.acted_at) as hour_num, COUNT(*) as cnt
		FROM request_approvals ra
		JOIN requests r ON r.id = ra.request_id
		WHERE ra.status = 'approved' AND ra.acted_at IS NOT NULL
		  AND %s
		GROUP BY HOUR(ra.acted_at)
		ORDER BY hour_num
	`, dateWhere(year, monthStart, monthEnd, "r"))
	rows, err := h.db.Query(query)
	if err != nil {
		return []hourlyApprovalItem{}
	}
	defer rows.Close()

	var result []hourlyApprovalItem
	for rows.Next() {
		var hour, cnt int
		if err := rows.Scan(&hour, &cnt); err != nil {
			continue
		}
		result = append(result, hourlyApprovalItem{Hour: hour, Count: cnt})
	}
	return result
}

func buildClassDistribution(h *LetterHandler, year, monthStart, monthEnd int) []classDistItem {
	query := fmt.Sprintf(`
		SELECT c.class_name, COUNT(*) as cnt
		FROM requests r
		JOIN request_students rs ON r.id = rs.request_id
		JOIN student_profiles sp ON rs.student_id = sp.id
		JOIN student_class_enrollments sce ON sp.id = sce.student_id AND sce.is_active = 1
		JOIN classes c ON sce.class_id = c.id
		WHERE r.deleted_at IS NULL AND r.status != 'draft'
		  AND %s
		GROUP BY c.id, c.class_name
		ORDER BY cnt DESC
		LIMIT 5
	`, dateWhere(year, monthStart, monthEnd, "r"))
	rows, err := h.db.Query(query)
	if err != nil {
		return []classDistItem{}
	}
	defer rows.Close()

	var result []classDistItem
	for rows.Next() {
		var name string
		var cnt int
		if err := rows.Scan(&name, &cnt); err != nil {
			continue
		}
		result = append(result, classDistItem{ClassName: name, Count: cnt})
	}
	return result
}

func buildTopStudents(h *LetterHandler, year, monthStart, monthEnd int) []topStudentItem {
	query := fmt.Sprintf(`
		SELECT sp.full_name, c.class_name, COUNT(*) as cnt
		FROM requests r
		JOIN request_students rs ON r.id = rs.request_id
		JOIN student_profiles sp ON rs.student_id = sp.id
		JOIN student_class_enrollments sce ON sp.id = sce.student_id AND sce.is_active = 1
		JOIN classes c ON sce.class_id = c.id
		WHERE r.deleted_at IS NULL AND r.status != 'draft'
		  AND %s
		GROUP BY sp.id, sp.full_name, c.class_name
		ORDER BY cnt DESC
		LIMIT 5`, dateWhere(year, monthStart, monthEnd, "r"))
	rows, err := h.db.Query(query)
	if err != nil {
		return []topStudentItem{}
	}
	defer rows.Close()

	var result []topStudentItem
	for rows.Next() {
		var name, className string
		var cnt int
		if err := rows.Scan(&name, &className, &cnt); err != nil {
			continue
		}
		result = append(result, topStudentItem{Name: name, Class: className, Count: cnt})
	}
	return result
}

func buildSLAMetrics(h *LetterHandler, year, monthStart, monthEnd int) (float64, float64) {
	reqJoin := fmt.Sprintf(`
		FROM request_approvals ra
		JOIN requests r ON r.id = ra.request_id
		WHERE ra.acted_at IS NOT NULL AND ra.status = 'approved' AND ra.approver_role != 'student'
		  AND ra.acted_at > ra.created_at
		  AND %s`, dateWhere(year, monthStart, monthEnd, "r"))

	var slaAvg sql.NullFloat64
	_ = h.db.QueryRow(`SELECT COALESCE(AVG((UNIX_TIMESTAMP(ra.acted_at) - UNIX_TIMESTAMP(ra.created_at)) / 60), 0) ` + reqJoin).Scan(&slaAvg)

	var totalCount, fastCount int
	_ = h.db.QueryRow(`SELECT COUNT(*) ` + reqJoin).Scan(&totalCount)
	_ = h.db.QueryRow(`SELECT COUNT(*) ` + reqJoin + ` AND (UNIX_TIMESTAMP(ra.acted_at) - UNIX_TIMESTAMP(ra.created_at)) / 60 <= 60`).Scan(&fastCount)

	avgVal := 0.0
	if slaAvg.Valid {
		avgVal = slaAvg.Float64
	}
	perfVal := 0.0
	if totalCount > 0 {
		perfVal = float64(fastCount) / float64(totalCount) * 100
	}
	return avgVal, perfVal
}

func buildViolations(h *LetterHandler, year, monthStart, monthEnd int) []violationItem {
	query := fmt.Sprintf(`
		SELECT
			sp.full_name,
			c.class_name,
			COUNT(*) as cnt,
			GROUP_CONCAT(DISTINCT rt.label SEPARATOR ', ') as types
		FROM requests r
		JOIN request_students rs ON r.id = rs.request_id
		JOIN student_profiles sp ON rs.student_id = sp.id
		JOIN student_class_enrollments sce ON sp.id = sce.student_id AND sce.is_active = 1
		JOIN classes c ON sce.class_id = c.id
		JOIN request_types rt ON r.request_type_id = rt.id
		WHERE r.deleted_at IS NULL
		  AND r.status NOT IN ('draft', 'cancelled')
		  AND rt.code IN ('izin_masuk', 'izin_keluar')
		  AND %s
		GROUP BY sp.id, sp.full_name, c.class_name
		HAVING cnt >= 3
		ORDER BY cnt DESC
		LIMIT 5`, dateWhere(year, monthStart, monthEnd, "r"))
	rows, err := h.db.Query(query)
	if err != nil {
		return []violationItem{}
	}
	defer rows.Close()

	var result []violationItem
	for rows.Next() {
		var name, className, types string
		var cnt int
		if err := rows.Scan(&name, &className, &cnt, &types); err != nil {
			continue
		}

		vType := "Frekuensi Izin Tinggi"
		if containsString(types, "Izin Keluar") {
			vType = "Izin Keluar di Tengah Jam Pelajaran Inti"
		} else if containsString(types, "Izin Masuk") {
			vType = "Izin Masuk Berulang di Luar Ketentuan"
		}

		rec := "Pemantauan Lebih Lanjut"
		if cnt >= 7 {
			rec = "Panggilan Wali Murid / Guru BK"
		} else if cnt >= 4 {
			rec = "Teguran Tertulis Wali Kelas"
		}

		result = append(result, violationItem{
			Name:           name,
			ClassName:      className,
			ViolationType:  vType,
			Frequency:      fmt.Sprintf("%dx Bulan Ini", cnt),
			Recommendation: rec,
		})
	}
	return result
}

func containsString(slice, target string) bool {
	return strings.Contains(slice, target)
}

func (h *LetterHandler) KepsekPending(c *gin.Context) {
	userID := toIntFromContext(c, "userId")
	if userID <= 0 {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var principalID int
	err := h.db.QueryRow(
		`SELECT id FROM principal_profiles WHERE user_id = ? AND active = 1 AND deleted_at IS NULL LIMIT 1`,
		userID,
	).Scan(&principalID)
	if err != nil {
		response.Success(c, http.StatusOK, "", gin.H{"data": []any{}, "currentPage": 1, "totalPages": 0, "totalItems": 0})
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
		  AND NOT EXISTS (
		      SELECT 1 FROM request_approvals ra2
		      WHERE ra2.request_id = r.id
		        AND ra2.approver_role = 'tu'
		        AND ra2.status = 'pending'
		        AND ra2.deleted_at IS NULL
		  )
	`, principalID).Scan(&totalItems); err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal memuat data")
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
		  AND NOT EXISTS (
		      SELECT 1 FROM request_approvals ra2
		      WHERE ra2.request_id = r.id
		        AND ra2.approver_role = 'tu'
		        AND ra2.status = 'pending'
		        AND ra2.deleted_at IS NULL
		  )
		ORDER BY r.submitted_at DESC
		LIMIT ? OFFSET ?
	`, principalID, limit, offset)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal memuat data")
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
			response.Error(c, http.StatusInternalServerError, "Gagal memproses data")
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
		response.Error(c, http.StatusInternalServerError, "Gagal memproses data")
		return
	}

	totalPages := totalItems / limit
	if totalItems%limit != 0 {
		totalPages++
	}

	response.Success(c, http.StatusOK, "", gin.H{
		"data":        items,
		"currentPage": page,
		"totalPages":  totalPages,
		"totalItems":  totalItems,
	})
}

func (h *LetterHandler) CreateStudent(c *gin.Context) { h.create(c) }
func (h *LetterHandler) CreateTeacher(c *gin.Context) { h.create(c) }

func (h *LetterHandler) create(c *gin.Context) {
	var req domain.LetterCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	userID := toIntFromContext(c, "userId")
	if userID <= 0 {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	id, err := h.service.Create(userID, req)
	if err != nil {
		if isValidationError(err) {
			response.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, "Gagal membuat surat")
		return
	}
	utils.LogActivity(h.db, int64(userID), "create_letter", "Pembuatan surat baru ID #"+strconv.Itoa(id), c.ClientIP(), c.Request.UserAgent())
	response.Success(c, http.StatusCreated, "", gin.H{"request_id": id})
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
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	page, limit := parsePagination(c)
	resp, err := h.service.ListGeneralDispensasi("teacher", userID, page, limit)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal memuat data")
		return
	}
	response.Success(c, http.StatusOK, "", resp)
}

func (h *LetterHandler) TeacherLetters(c *gin.Context) {
	userID := toIntFromContext(c, "userId")
	if userID <= 0 {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	page, limit := parsePagination(c)
	resp, err := h.service.ListTeacherLetters(userID, page, limit)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal memuat data")
		return
	}
	response.Success(c, http.StatusOK, "", resp)
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
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	page, limit := parsePagination(c)
	resp, err := h.service.ListForStudent(userID, typeKey, page, limit)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal memuat data")
		return
	}
	response.Success(c, http.StatusOK, "", resp)
}

func (h *LetterHandler) listTeacher(c *gin.Context, typeKey string) {
	userID := toIntFromContext(c, "userId")
	if userID <= 0 {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	page, limit := parsePagination(c)
	resp, err := h.service.ListForTeacherScoped(userID, typeKey, page, limit)
	if err != nil {
		if err.Error() == "forbidden: no active roles" {
			response.Error(c, http.StatusForbidden, "Anda tidak memiliki peran aktif")
			return
		}
		response.Error(c, http.StatusInternalServerError, "Gagal memuat data")
		return
	}
	response.Success(c, http.StatusOK, "", resp)
}

func (h *LetterHandler) TeacherPending(c *gin.Context) {
	userID := toIntFromContext(c, "userId")
	if userID <= 0 {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	page, limit := parsePagination(c)
	resp, err := h.service.ListPendingForTeacher(userID, page, limit)
	if err != nil {
		if err.Error() == "forbidden: no active roles" {
			response.Error(c, http.StatusForbidden, "Anda tidak memiliki peran aktif")
			return
		}
		response.Error(c, http.StatusInternalServerError, "Gagal memuat data")
		return
	}
	response.Success(c, http.StatusOK, "", resp)
}

func (h *LetterHandler) TeacherStats(c *gin.Context) {
	userID := toIntFromContext(c, "userId")
	if userID <= 0 {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	stats, err := h.service.GetTeacherStats(userID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal memuat statistik")
		return
	}
	response.Success(c, http.StatusOK, "", stats)
}
