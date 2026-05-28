package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Refliqx/backend-eletter/internal/domain"
)

type LetterRepository interface {
	domain.LetterRepository
}

type letterRepository struct {
	db         *sql.DB
	schoolCode string
	publisher  NotificationPublisher
}

func NewLetterRepository(db *sql.DB, schoolCode string, publisher NotificationPublisher) LetterRepository {
	return &letterRepository{db: db, schoolCode: schoolCode, publisher: publisher}
}

func (r *letterRepository) CreateLetter(userID int, req domain.LetterCreateRequest) (int, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	requestNumber, academicYearID, err := generateRequestNumber(tx, req.TypeID, r.schoolCode)
	if err != nil {
		return 0, err
	}

	res, err := tx.Exec(
		`INSERT INTO requests (request_number, academic_year_id, request_type_id, requester_user_id, reason, start_time, end_time, request_date, submitted_at, status, current_step)
		 VALUES (?,?,?,?,?,?,?,?,NOW(),'pending',1)`,
		requestNumber, academicYearID, req.TypeID, userID, req.Description, req.StartTime, req.EndTime, resolveRequestDate(req.RequestDate, req.StartTime, req.EndTime),
	)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	requestID := int(id)

	if len(req.Students) > 0 {
		for _, studentID := range req.Students {
			if _, err := tx.Exec(`INSERT INTO request_students (request_id, student_id) VALUES (?, ?)`, requestID, studentID); err != nil {
				return 0, err
			}
		}
	}

	// --- Approval Orchestration ---
	firstPendingStep, firstPendingApproverUserID, err := r.createApprovalSteps(tx, int64(requestID), req.TypeID, userID, int64(academicYearID))
	if err != nil {
		return 0, fmt.Errorf("approval orchestration: %w", err)
	}
	if firstPendingStep > 0 {
		if _, err := tx.Exec(`UPDATE requests SET current_step = ? WHERE id = ?`, firstPendingStep, requestID); err != nil {
			return 0, err
		}
	}

	body := fmt.Sprintf("Permohonan Anda dengan nomor %s telah dikirim dan menunggu persetujuan.", requestNumber)
	requestID64 := int64(requestID)
	if err := createNotificationTx(tx, int64(userID), "request_submitted", "Permohonan terkirim", &body, &requestID64, nil); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	if r.publisher != nil {
		r.publisher.Publish(userID, "notifications:refresh")
		if firstPendingApproverUserID != nil {
			r.publisher.Publish(int(*firstPendingApproverUserID), "notifications:refresh")
		}
	}

	return requestID, nil
}

// createApprovalSteps reads flow templates, resolves approvers, inserts approval rows,
// and sends a notification to the first pending approver. Returns the first pending step number.
func (r *letterRepository) createApprovalSteps(tx *sql.Tx, requestID int64, requestTypeID int, userID int, academicYearID int64) (int, *int64, error) {
	// 1. Read flow templates
	rows, err := r.db.Query(
		`SELECT step_no, approver_role, is_required, skip_if_no_schedule
		 FROM approval_flow_templates WHERE request_type_id = ? ORDER BY step_no ASC`, requestTypeID)
	if err != nil {
		return 1, nil, err
	}
	defer rows.Close()

	type tmpl struct {
		StepNo           int
		ApproverRole     string
		IsRequired       bool
		SkipIfNoSchedule bool
	}
	var templates []tmpl
	for rows.Next() {
		var t tmpl
		if err := rows.Scan(&t.StepNo, &t.ApproverRole, &t.IsRequired, &t.SkipIfNoSchedule); err != nil {
			return 1, nil, err
		}
		templates = append(templates, t)
	}
	if err := rows.Err(); err != nil {
		return 1, nil, err
	}
	if len(templates) == 0 {
		return 1, nil, nil
	}

	// 2. Gather student context
	var classID sql.NullInt64
	r.db.QueryRow(
		`SELECT sce.class_id FROM student_profiles sp
		 JOIN student_class_enrollments sce ON sce.student_id = sp.id AND sce.is_active = 1
		 WHERE sp.user_id = ? LIMIT 1`, userID).Scan(&classID)

	var majorID sql.NullInt64
	if classID.Valid {
		r.db.QueryRow(`SELECT major_id FROM classes WHERE id = ?`, classID.Int64).Scan(&majorID)
	}

	var studentSig sql.NullString
	r.db.QueryRow(`SELECT signature_url FROM student_profiles WHERE user_id = ?`, userID).Scan(&studentSig)

	// Determine request_date for schedule lookup
	requestDate := time.Now().Format("2006-01-02")

	// 3. Resolve each step
	type resolvedStep struct {
		StepNo              int
		ApproverRole        string
		ApproverTeacherID   *int64
		ApproverPrincipalID *int64
		Status              string
		SignatureURL        *string
		ScheduleID          *int64
		Notes               *string
	}

	var steps []resolvedStep
	firstPendingStep := -1

	for _, t := range templates {
		s := resolvedStep{
			StepNo:       t.StepNo,
			ApproverRole: t.ApproverRole,
			Status:       "pending",
		}

		switch t.ApproverRole {
		case "student":
			s.Status = "approved"
			if studentSig.Valid {
				s.SignatureURL = &studentSig.String
			}
			note := "Tanda tangan digital siswa — otomatis dari profil."
			s.Notes = &note

		case "wali_kelas":
			if classID.Valid {
				var tid int64
				if err := r.db.QueryRow(
					`SELECT teacher_id FROM class_homeroom_assignments
					 WHERE class_id = ? AND academic_year_id = ? AND is_active = 1 LIMIT 1`,
					classID.Int64, academicYearID).Scan(&tid); err == nil {
					s.ApproverTeacherID = &tid
				}
			}

		case "guru_mapel":
			if classID.Valid {
				tid, sid := r.findGuruMapelSchedule(classID.Int64, academicYearID, requestDate)
				if t.SkipIfNoSchedule && tid == nil {
					s.Status = "skipped"
					note := "Auto-skip: tidak ada jadwal guru mapel yang overlap."
					s.Notes = &note
				} else {
					s.ApproverTeacherID = tid
					s.ScheduleID = sid
				}
			} else if t.SkipIfNoSchedule {
				s.Status = "skipped"
				note := "Auto-skip: tidak ada jadwal guru mapel yang overlap."
				s.Notes = &note
			}

		case "kapro":
			if majorID.Valid {
				var tid int64
				if err := r.db.QueryRow(
					`SELECT teacher_id FROM major_head_assignments
					 WHERE major_id = ? AND academic_year_id = ? AND is_active = 1 LIMIT 1`,
					majorID.Int64, academicYearID).Scan(&tid); err == nil {
					s.ApproverTeacherID = &tid
				}
			}

		case "tatib":
			var tid int64
			if err := r.db.QueryRow(
				`SELECT teacher_id FROM teacher_roles WHERE role_name = 'tatib' AND status = 'active' LIMIT 1`).Scan(&tid); err == nil {
				s.ApproverTeacherID = &tid
			}

		case "kepala_sekolah":
			var pid int64
			if err := r.db.QueryRow(
				`SELECT id FROM principal_profiles WHERE active = 1 AND deleted_at IS NULL LIMIT 1`).Scan(&pid); err == nil {
				s.ApproverPrincipalID = &pid
			}
		}

		if firstPendingStep == -1 && s.Status == "pending" {
			firstPendingStep = t.StepNo
		}
		steps = append(steps, s)
	}

	// 4. Insert all approval rows
	for _, s := range steps {
		var actedAt interface{}
		if s.Status == "approved" || s.Status == "skipped" {
			actedAt = time.Now()
		}
		_, err := tx.Exec(
			`INSERT INTO request_approvals
			 (request_id, step_no, approver_role, approver_teacher_id, approver_principal_id, status, signature_url, schedule_id, acted_at, notes)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			requestID, s.StepNo, s.ApproverRole, s.ApproverTeacherID, s.ApproverPrincipalID,
			s.Status, s.SignatureURL, s.ScheduleID, actedAt, s.Notes,
		)
		if err != nil {
			return 1, nil, fmt.Errorf("insert approval step %d (%s): %w", s.StepNo, s.ApproverRole, err)
		}
	}

	// 5. Notify first pending approver
	if firstPendingStep >= 0 {
		for _, s := range steps {
			if s.StepNo == firstPendingStep && s.Status == "pending" {
				var approverUserID int64
				var resolved bool
				if s.ApproverTeacherID != nil {
					if err := r.db.QueryRow(`SELECT user_id FROM teacher_profiles WHERE id = ?`, *s.ApproverTeacherID).Scan(&approverUserID); err == nil {
						resolved = true
					}
				} else if s.ApproverPrincipalID != nil {
					if err := r.db.QueryRow(`SELECT user_id FROM principal_profiles WHERE id = ?`, *s.ApproverPrincipalID).Scan(&approverUserID); err == nil {
						resolved = true
					}
				}
				if resolved {
					body := "Ada permohonan izin yang memerlukan persetujuan Anda."
					if err := createNotificationTx(tx, approverUserID, "new_request", "Permohonan Izin Baru", &body, &requestID, nil); err != nil {
						return 1, nil, err
					}
					return firstPendingStep, &approverUserID, nil
				}
				break
			}
		}
	}

	if firstPendingStep == -1 {
		firstPendingStep = 1
	}
	return firstPendingStep, nil, nil
}

// findGuruMapelSchedule finds a teacher with a schedule on the given date for the class.
func (r *letterRepository) findGuruMapelSchedule(classID, academicYearID int64, requestDate string) (*int64, *int64) {
	t, err := time.Parse("2006-01-02", requestDate)
	if err != nil {
		return nil, nil
	}
	day := dayOfWeekIndo(t.Weekday())
	if day == "" {
		return nil, nil // weekend
	}

	var teacherID, scheduleID int64
	err = r.db.QueryRow(
		`SELECT teacher_id, id FROM schedules
		 WHERE class_id = ? AND academic_year_id = ? AND day_of_week = ? AND is_active = 1
		 LIMIT 1`, classID, academicYearID, day).Scan(&teacherID, &scheduleID)
	if err != nil {
		return nil, nil
	}
	return &teacherID, &scheduleID
}

func dayOfWeekIndo(d time.Weekday) string {
	switch d {
	case time.Monday:
		return "senin"
	case time.Tuesday:
		return "selasa"
	case time.Wednesday:
		return "rabu"
	case time.Thursday:
		return "kamis"
	case time.Friday:
		return "jumat"
	default:
		return ""
	}
}

func (r *letterRepository) ListLettersForUser(userID int, typeKey string, page, limit int) (*domain.PaginatedLetterResponse, error) {
	offset := (page - 1) * limit
	var totalItems int
	err := r.db.QueryRow(`
		SELECT COUNT(*)
		FROM requests r
		JOIN request_types rt ON rt.id = r.request_type_id
		WHERE r.requester_user_id = ? AND rt.code = ? AND r.deleted_at IS NULL
	`, userID, typeKey).Scan(&totalItems)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(`
		SELECT r.id, rt.label, r.reason, r.status, r.request_date, r.created_at, r.updated_at, r.start_time, r.end_time,
		       COALESCE(sp.full_name,''), COALESCE(c.class_name,'-'), COALESCE(sp.student_code,'-'), COALESCE(u.email,'-')
		FROM requests r
		JOIN request_types rt ON rt.id = r.request_type_id
		JOIN users u ON u.id = r.requester_user_id
		LEFT JOIN student_profiles sp ON sp.user_id = u.id
		LEFT JOIN student_class_enrollments sce ON sce.student_id = sp.id AND sce.is_active = 1
		LEFT JOIN classes c ON c.id = sce.class_id
		WHERE r.requester_user_id = ? AND rt.code = ? AND r.deleted_at IS NULL
		ORDER BY r.created_at DESC
		LIMIT ? OFFSET ?
	`, userID, typeKey, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanLetterRows(rows)
	if err != nil {
		return nil, err
	}

	totalPages := totalItems / limit
	if totalItems%limit != 0 {
		totalPages++
	}

	return &domain.PaginatedLetterResponse{
		Data:        items,
		CurrentPage: page,
		TotalPages:  totalPages,
		TotalItems:  totalItems,
	}, nil
}

func (r *letterRepository) ListLettersForTeacher(typeKey string, page, limit int) (*domain.PaginatedLetterResponse, error) {
	offset := (page - 1) * limit
	var totalItems int
	err := r.db.QueryRow(`
		SELECT COUNT(*)
		FROM requests r
		JOIN request_types rt ON rt.id = r.request_type_id
		WHERE rt.code = ? AND r.deleted_at IS NULL
	`, typeKey).Scan(&totalItems)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(`
		SELECT r.id, rt.label, r.reason, r.status, r.request_date, r.created_at, r.updated_at, r.start_time, r.end_time,
		       COALESCE(sp.full_name,''), COALESCE(c.class_name,'-'), COALESCE(sp.student_code,'-'), COALESCE(u.email,'-')
		FROM requests r
		JOIN request_types rt ON rt.id = r.request_type_id
		JOIN users u ON u.id = r.requester_user_id
		LEFT JOIN student_profiles sp ON sp.user_id = u.id
		LEFT JOIN student_class_enrollments sce ON sce.student_id = sp.id AND sce.is_active = 1
		LEFT JOIN classes c ON c.id = sce.class_id
		WHERE rt.code = ? AND r.deleted_at IS NULL
		ORDER BY r.created_at DESC
		LIMIT ? OFFSET ?
	`, typeKey, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanLetterRows(rows)
	if err != nil {
		return nil, err
	}

	totalPages := totalItems / limit
	if totalItems%limit != 0 {
		totalPages++
	}

	return &domain.PaginatedLetterResponse{
		Data:        items,
		CurrentPage: page,
		TotalPages:  totalPages,
		TotalItems:  totalItems,
	}, nil
}

func scanLetterRows(rows *sql.Rows) ([]domain.LetterListItem, error) {
	items := make([]domain.LetterListItem, 0)
	for rows.Next() {
		var (
			id, title, description, status, fullName, className, nisn, email string
			requestDate, createdAt, updatedAt                                time.Time
			startTime, endTime                                               sql.NullTime
		)
		var idInt int
		if err := rows.Scan(&idInt, &title, &description, &status, &requestDate, &createdAt, &updatedAt, &startTime, &endTime, &fullName, &className, &nisn, &email); err != nil {
			return nil, err
		}
		statusUI := mapStatus(status)
		submitted := requestDate.Format("2006-01-02")
		item := domain.LetterListItem{
			ID:          idInt,
			Title:       coalesceTitle(title),
			Status:      statusUI,
			Date:        submitted,
			Description: description,
			StudentInfo: domain.StudentInfoDTO{
				Name:  fullName,
				Class: className,
				NISN:  nisn,
				Email: email,
			},
			RequestInfo: domain.RequestInfoDTO{
				SubmittedDate: submitted,
				ApprovedDate:  approvedDate(status, updatedAt),
				Notes:         description,
			},
			TimeInfo: domain.TimeInfoDTO{
				StartTime: formatNullableTime(startTime),
				EndTime:   formatNullableTime(endTime),
			},
		}
		items = append(items, item)
		_ = id
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func resolveRequestDate(requestDate string, startTime string, endTime string) string {
	trimmed := strings.TrimSpace(requestDate)
	if trimmed != "" {
		return trimmed
	}
	for _, candidate := range []string{startTime, endTime} {
		candidate = strings.TrimSpace(candidate)
		if len(candidate) >= 10 {
			return candidate[:10]
		}
	}
	return ""
}

func mapStatus(status string) string {
	switch status {
	case "approved":
		return "disetujui"
	case "rejected", "cancelled":
		return "ditolak"
	default:
		return "menunggu"
	}
}

func approvedDate(status string, updatedAt time.Time) string {
	if status == "approved" {
		return updatedAt.Format("2006-01-02")
	}
	return "-"
}

func formatNullableTime(t sql.NullTime) string {
	if !t.Valid {
		return "-"
	}
	return t.Time.Format("15:04:05")
}

func coalesceTitle(v string) string {
	if v == "" {
		return "Surat"
	}
	return v
}

func (r *letterRepository) ListGeneralDispensasi(userRole string, userID int, page, limit int) (*domain.PaginatedLetterResponse, error) {
	offset := (page - 1) * limit
	var totalItems int

	var countQuery string
	var selectQuery string

	if userRole == "teacher" {
		scopeFilter, err := BuildRBACScopeFilter(r.db, userID)
		if err != nil {
			return nil, err
		}

		countQuery = fmt.Sprintf(`
			SELECT COUNT(DISTINCT r.id)
			FROM requests r
			JOIN request_types rt ON rt.id = r.request_type_id
			JOIN request_students rs ON rs.request_id = r.id
			JOIN student_profiles sp ON sp.id = rs.student_id
			JOIN student_class_enrollments sce ON sce.student_id = sp.id AND sce.is_active = 1
			WHERE rt.code = 'dispensasi' AND r.deleted_at IS NULL AND (%s)
		`, scopeFilter)

		selectQuery = fmt.Sprintf(`
			SELECT DISTINCT r.id, rt.label, r.reason, r.status, r.created_at, r.updated_at, r.start_time, r.end_time,
			       COALESCE(tp.full_name,''), '-', COALESCE(tp.employee_code,'-'), COALESCE(u.email,'-')
			FROM requests r
			JOIN request_types rt ON rt.id = r.request_type_id
			JOIN users u ON u.id = r.requester_user_id
			LEFT JOIN teacher_profiles tp ON tp.user_id = u.id
			JOIN request_students rs ON rs.request_id = r.id
			JOIN student_profiles sp ON sp.id = rs.student_id
			JOIN student_class_enrollments sce ON sce.student_id = sp.id AND sce.is_active = 1
			WHERE rt.code = 'dispensasi' AND r.deleted_at IS NULL AND (%s)
			ORDER BY r.created_at DESC
			LIMIT ? OFFSET ?
		`, scopeFilter)
	} else {
		countQuery = `
			SELECT COUNT(DISTINCT r.id)
			FROM requests r
			JOIN request_types rt ON rt.id = r.request_type_id
			WHERE rt.code = 'dispensasi' AND r.deleted_at IS NULL
		`

		selectQuery = `
			SELECT DISTINCT r.id, rt.label, r.reason, r.status, r.created_at, r.updated_at, r.start_time, r.end_time,
			       COALESCE(tp.full_name,''), '-', COALESCE(tp.employee_code,'-'), COALESCE(u.email,'-')
			FROM requests r
			JOIN request_types rt ON rt.id = r.request_type_id
			JOIN users u ON u.id = r.requester_user_id
			LEFT JOIN teacher_profiles tp ON tp.user_id = u.id
			WHERE rt.code = 'dispensasi' AND r.deleted_at IS NULL
			ORDER BY r.created_at DESC
			LIMIT ? OFFSET ?
		`
	}

	err := r.db.QueryRow(countQuery).Scan(&totalItems)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(selectQuery, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanLetterRows(rows)
	if err != nil {
		return nil, err
	}

	totalPages := totalItems / limit
	if totalItems%limit != 0 {
		totalPages++
	}

	return &domain.PaginatedLetterResponse{
		Data:        items,
		CurrentPage: page,
		TotalPages:  totalPages,
		TotalItems:  totalItems,
	}, nil
}

func (r *letterRepository) GetTeacherActiveRoles(userID int) ([]domain.TeacherRole, error) {
	rows, err := r.db.Query(`
		SELECT tr.role_name
		FROM teacher_roles tr
		JOIN teacher_profiles tp ON tp.id = tr.teacher_id
		WHERE tp.user_id = ? AND tr.status = 'active'
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []domain.TeacherRole
	for rows.Next() {
		var r domain.TeacherRole
		if err := rows.Scan(&r.RoleName); err != nil {
			return nil, err
		}
		roles = append(roles, r)
	}
	return roles, rows.Err()
}

func (r *letterRepository) ListLettersForTeacherScoped(userID int, typeKey string, page, limit int) (*domain.PaginatedLetterResponse, error) {
	offset := (page - 1) * limit

	scopeFilter, err := BuildRBACScopeFilter(r.db, userID)
	if err != nil {
		return nil, err
	}

	countQuery := fmt.Sprintf(`
		SELECT COUNT(DISTINCT r.id)
		FROM requests r
		JOIN request_types rt ON rt.id = r.request_type_id
		JOIN users u ON u.id = r.requester_user_id
		LEFT JOIN student_profiles sp ON sp.user_id = u.id
		LEFT JOIN student_class_enrollments sce ON sce.student_id = sp.id AND sce.is_active = 1
		WHERE rt.code = ? AND r.deleted_at IS NULL AND (%s)
	`, scopeFilter)

	var totalItems int
	if err := r.db.QueryRow(countQuery, typeKey).Scan(&totalItems); err != nil {
		return nil, err
	}

	rows, err := r.db.Query(fmt.Sprintf(`
		SELECT DISTINCT r.id, rt.label, r.reason, r.status, r.created_at, r.updated_at, r.start_time, r.end_time,
		       COALESCE(sp.full_name,''), COALESCE(c.class_name,'-'), COALESCE(sp.student_code,'-'), COALESCE(u.email,'-')
		FROM requests r
		JOIN request_types rt ON rt.id = r.request_type_id
		JOIN users u ON u.id = r.requester_user_id
		LEFT JOIN student_profiles sp ON sp.user_id = u.id
		LEFT JOIN student_class_enrollments sce ON sce.student_id = sp.id AND sce.is_active = 1
		LEFT JOIN classes c ON c.id = sce.class_id
		WHERE rt.code = ? AND r.deleted_at IS NULL AND (%s)
		ORDER BY r.created_at DESC
		LIMIT ? OFFSET ?
	`, scopeFilter), typeKey, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanLetterRows(rows)
	if err != nil {
		return nil, err
	}

	totalPages := totalItems / limit
	if totalItems%limit != 0 {
		totalPages++
	}
	return &domain.PaginatedLetterResponse{
		Data:        items,
		CurrentPage: page,
		TotalPages:  totalPages,
		TotalItems:  totalItems,
	}, nil
}

func (r *letterRepository) ListPendingForTeacher(userID int, page, limit int) (*domain.PaginatedLetterResponse, error) {
	offset := (page - 1) * limit

	countQuery := `
		SELECT COUNT(*)
		FROM v_pending_approvals_for_teacher v
		JOIN teacher_profiles tp ON tp.id = v.approver_teacher_id
		WHERE tp.user_id = ?
	`
	var totalItems int
	if err := r.db.QueryRow(countQuery, userID).Scan(&totalItems); err != nil {
		return nil, err
	}

	rows, err := r.db.Query(`
		SELECT v.request_id, v.request_type_label, v.reason, 'pending', v.submitted_at, v.submitted_at, v.start_time, v.end_time,
		       COALESCE(v.requester_name,''), '-', '-', COALESCE(v.requester_email,'-')
		FROM v_pending_approvals_for_teacher v
		JOIN teacher_profiles tp ON tp.id = v.approver_teacher_id
		WHERE tp.user_id = ?
		ORDER BY v.submitted_at DESC
		LIMIT ? OFFSET ?
	`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanLetterRows(rows)
	if err != nil {
		return nil, err
	}

	totalPages := totalItems / limit
	if totalItems%limit != 0 {
		totalPages++
	}
	return &domain.PaginatedLetterResponse{
		Data:        items,
		CurrentPage: page,
		TotalPages:  totalPages,
		TotalItems:  totalItems,
	}, nil
}

func (r *letterRepository) ListTeacherLetters(userID int, page, limit int) (*domain.PaginatedLetterResponse, error) {
	offset := (page - 1) * limit
	var totalItems int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM requests r WHERE r.requester_user_id = ? AND r.deleted_at IS NULL`, userID).Scan(&totalItems)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(`
		SELECT r.id, rt.label, r.reason, r.status, r.created_at, r.updated_at, r.start_time, r.end_time,
		       COALESCE(tp.full_name,''), '-', COALESCE(tp.employee_code,'-'), COALESCE(u.email,'-')
		FROM requests r
		JOIN request_types rt ON rt.id = r.request_type_id
		JOIN users u ON u.id = r.requester_user_id
		LEFT JOIN teacher_profiles tp ON tp.user_id = u.id
		WHERE r.requester_user_id = ? AND r.deleted_at IS NULL
		ORDER BY r.created_at DESC
		LIMIT ? OFFSET ?
	`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanLetterRows(rows)
	if err != nil {
		return nil, err
	}

	totalPages := totalItems / limit
	if totalItems%limit != 0 {
		totalPages++
	}

	return &domain.PaginatedLetterResponse{
		Data:        items,
		CurrentPage: page,
		TotalPages:  totalPages,
		TotalItems:  totalItems,
	}, nil
}
