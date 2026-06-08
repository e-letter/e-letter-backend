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

// resolvedStep holds the resolved data for a single approval step before insertion.
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

	// Batch INSERT for request_students — single round trip instead of N.
	if len(req.Students) > 0 {
		if err := batchInsertRequestStudents(tx, requestID, req.Students); err != nil {
			return 0, err
		}
	} else {
		// Fallback: check if the requester has a student profile, and insert their student ID.
		var studentID int
		err := tx.QueryRow(`SELECT id FROM student_profiles WHERE user_id = ?`, userID).Scan(&studentID)
		if err == nil {
			if _, err := tx.Exec(`INSERT INTO request_students (request_id, student_id) VALUES (?, ?)`, requestID, studentID); err != nil {
				return 0, err
			}
		} else if err != sql.ErrNoRows {
			return 0, err
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
	if err := createNotificationTx(tx, int64(userID), "new_request", "Permohonan terkirim", &body, &requestID64, nil); err != nil {
		return 0, err
	}

	// Notify each student who is the subject of the request (dispensasi from teacher).
	// Resolve student_profiles.id -> user_id and create a notification for each.
	var studentUserIDs []int
	if len(req.Students) > 0 {
		placeholders := make([]string, len(req.Students))
		args := make([]any, len(req.Students))
		for i, sid := range req.Students {
			placeholders[i] = "?"
			args[i] = sid
		}
		sRows, err := tx.Query(`SELECT id, user_id FROM student_profiles WHERE id IN (`+strings.Join(placeholders, ",")+`)`, args...)
		if err == nil {
			for sRows.Next() {
				var spID, suID int
				if err := sRows.Scan(&spID, &suID); err == nil {
					studentUserIDs = append(studentUserIDs, suID)
				}
			}
			sRows.Close()
		}
	}

	// Get teacher/requester name for notification title
	var teacherName string
	_ = tx.QueryRow(`SELECT full_name FROM teacher_profiles WHERE user_id = ?`, userID).Scan(&teacherName)
	if teacherName == "" {
		_ = tx.QueryRow(`SELECT full_name FROM principal_profiles WHERE user_id = ?`, userID).Scan(&teacherName)
	}
	if teacherName == "" {
		teacherName = "Guru"
	}

	for _, suID := range studentUserIDs {
		studentTitle := "Permohonan Dispensasi"
		studentBody := fmt.Sprintf("Pengajuan surat dispensasi oleh %s, Status saat ini: menunggu persetujuan", teacherName)
		if err := createNotificationTx(tx, int64(suID), "new_request", studentTitle, &studentBody, &requestID64, nil); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	notifyUsers := []int{userID}
	if firstPendingApproverUserID != nil {
		notifyUsers = append(notifyUsers, int(*firstPendingApproverUserID))
	}
	notifyUsers = append(notifyUsers, studentUserIDs...)
	if r.publisher != nil {
		for _, uid := range notifyUsers {
			r.publisher.Publish(uid, "notifications:refresh")
		}
	}

	return requestID, nil
}

// batchInsertRequestStudents inserts all student IDs in a single multi-row INSERT.
func batchInsertRequestStudents(tx *sql.Tx, requestID int, studentIDs []int) error {
	if len(studentIDs) == 0 {
		return nil
	}
	// Build multi-row VALUES clause: (?,?),(?,?),...
	valueStrings := make([]string, 0, len(studentIDs))
	valueArgs := make([]any, 0, len(studentIDs)*2)
	for _, sid := range studentIDs {
		valueStrings = append(valueStrings, "(?, ?)")
		valueArgs = append(valueArgs, requestID, sid)
	}
	query := fmt.Sprintf("INSERT INTO request_students (request_id, student_id) VALUES %s", strings.Join(valueStrings, ","))
	_, err := tx.Exec(query, valueArgs...)
	return err
}

// createApprovalSteps reads flow templates, resolves approvers, inserts approval rows,
// and sends a notification to the first pending approver. Returns the first pending step number.
//
// Optimization: all reference data (role assignments, schedules, profiles) is preloaded ONCE
// before the template loop, eliminating N+1 query patterns. All queries use tx for transactional consistency.
func (r *letterRepository) createApprovalSteps(tx *sql.Tx, requestID int64, requestTypeID int, userID int, academicYearID int64) (int, *int64, error) {
	// 1. Read flow templates (via tx, not r.db)
	rows, err := tx.Query(
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

	// 2. Gather student context (via tx)
	var classID sql.NullInt64
	err = tx.QueryRow(
		`SELECT sce.class_id FROM student_profiles sp
		 JOIN student_class_enrollments sce ON sce.student_id = sp.id AND sce.is_active = 1
		 WHERE sp.user_id = ? LIMIT 1`, userID).Scan(&classID)
	if err != nil && err != sql.ErrNoRows {
		return 1, nil, fmt.Errorf("resolve student class: %w", err)
	}

	var majorID sql.NullInt64
	if classID.Valid {
		err = tx.QueryRow(`SELECT major_id FROM classes WHERE id = ?`, classID.Int64).Scan(&majorID)
		if err != nil && err != sql.ErrNoRows {
			return 1, nil, fmt.Errorf("resolve major: %w", err)
		}
	}

	var studentSig sql.NullString
	err = tx.QueryRow(`SELECT signature_url FROM student_profiles WHERE user_id = ?`, userID).Scan(&studentSig)
	if err != nil && err != sql.ErrNoRows {
		return 1, nil, fmt.Errorf("resolve student signature: %w", err)
	}

	// 3. Preload ALL reference data ONCE — eliminates N+1 inside the template loop.
	//    These maps are keyed by the relevant ID for O(1) lookup.

	// homeroomMap: classID -> teacherID (for wali_kelas resolution)
	homeroomMap := make(map[int64]int64)
	if classID.Valid {
		var hid int64
		if err := tx.QueryRow(
			`SELECT teacher_id FROM class_homeroom_assignments
			 WHERE class_id = ? AND academic_year_id = ? AND is_active = 1 LIMIT 1`,
			classID.Int64, academicYearID).Scan(&hid); err == nil {
			homeroomMap[classID.Int64] = hid
		}
	}

	// kaproMap: majorID -> teacherID (for kapro resolution)
	kaproMap := make(map[int64]int64)
	if majorID.Valid {
		var kid int64
		if err := tx.QueryRow(
			`SELECT teacher_id FROM major_head_assignments
			 WHERE major_id = ? AND academic_year_id = ? AND is_active = 1 LIMIT 1`,
			majorID.Int64, academicYearID).Scan(&kid); err == nil {
			kaproMap[majorID.Int64] = kid
		}
	}

	// tatibTeacherID: any active tatib teacher
	var tatibTeacherID sql.NullInt64
	{
		var tid int64
		if err := tx.QueryRow(
			`SELECT teacher_id FROM teacher_roles WHERE role_name = 'tatib' AND status = 'active' LIMIT 1`,
		).Scan(&tid); err == nil {
			tatibTeacherID = sql.NullInt64{Int64: tid, Valid: true}
		}
	}

	// kepsekPrincipalID: active principal
	var kepsekPrincipalID sql.NullInt64
	{
		var pid int64
		if err := tx.QueryRow(
			`SELECT id FROM principal_profiles WHERE active = 1 AND deleted_at IS NULL LIMIT 1`,
		).Scan(&pid); err == nil {
			kepsekPrincipalID = sql.NullInt64{Int64: pid, Valid: true}
		}
	}

	// Determine request_date for schedule lookup
	requestDate := time.Now().Format("2006-01-02")

	// 4. Resolve each step using preloaded maps
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
				if tid, ok := homeroomMap[classID.Int64]; ok {
					s.ApproverTeacherID = &tid
				}
			}

		case "guru_mapel":
			if classID.Valid {
				tid, sid := r.findGuruMapelSchedule(tx, classID.Int64, academicYearID, requestDate)
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
				if tid, ok := kaproMap[majorID.Int64]; ok {
					s.ApproverTeacherID = &tid
				}
			}

		case "tatib":
			if tatibTeacherID.Valid {
				tid := tatibTeacherID.Int64
				s.ApproverTeacherID = &tid
			}

		case "kepala_sekolah":
			if kepsekPrincipalID.Valid {
				pid := kepsekPrincipalID.Int64
				s.ApproverPrincipalID = &pid
			}
		}

		if firstPendingStep == -1 && s.Status == "pending" {
			firstPendingStep = t.StepNo
		}
		steps = append(steps, s)
	}

	// 5. Batch INSERT all approval rows — single round trip instead of N.
	if len(steps) > 0 {
		if err := batchInsertApprovalSteps(tx, requestID, steps); err != nil {
			return 1, nil, fmt.Errorf("batch insert approval steps: %w", err)
		}
	}

	// 6. Notify first pending approver
	if firstPendingStep >= 0 {
		for _, s := range steps {
			if s.StepNo == firstPendingStep && s.Status == "pending" {
				var approverUserID int64
				var resolved bool
				if s.ApproverTeacherID != nil {
					if err := tx.QueryRow(`SELECT user_id FROM teacher_profiles WHERE id = ?`, *s.ApproverTeacherID).Scan(&approverUserID); err == nil {
						resolved = true
					}
				} else if s.ApproverPrincipalID != nil {
					if err := tx.QueryRow(`SELECT user_id FROM principal_profiles WHERE id = ?`, *s.ApproverPrincipalID).Scan(&approverUserID); err == nil {
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

// batchInsertApprovalSteps inserts all resolved approval steps in a single multi-row INSERT.
func batchInsertApprovalSteps(tx *sql.Tx, requestID int64, steps []resolvedStep) error {
	if len(steps) == 0 {
		return nil
	}
	valueStrings := make([]string, 0, len(steps))
	valueArgs := make([]any, 0, len(steps)*10)
	for _, s := range steps {
		valueStrings = append(valueStrings, "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		var actedAt any
		if s.Status == "approved" || s.Status == "skipped" {
			actedAt = time.Now()
		}
		valueArgs = append(valueArgs,
			requestID, s.StepNo, s.ApproverRole, s.ApproverTeacherID, s.ApproverPrincipalID,
			s.Status, s.SignatureURL, s.ScheduleID, actedAt, s.Notes,
		)
	}
	query := fmt.Sprintf(
		`INSERT INTO request_approvals
		 (request_id, step_no, approver_role, approver_teacher_id, approver_principal_id, status, signature_url, schedule_id, acted_at, notes)
		 VALUES %s`, strings.Join(valueStrings, ","))
	_, err := tx.Exec(query, valueArgs...)
	return err
}

// findGuruMapelSchedule finds a teacher with a schedule on the given date for the class.
// Uses tx for transactional consistency instead of r.db.
func (r *letterRepository) findGuruMapelSchedule(tx *sql.Tx, classID, academicYearID int64, requestDate string) (*int64, *int64) {
	t, err := time.Parse("2006-01-02", requestDate)
	if err != nil {
		return nil, nil
	}
	day := dayOfWeekIndo(t.Weekday())
	if day == "" {
		return nil, nil // weekend
	}

	var teacherID, scheduleID int64
	err = tx.QueryRow(
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
		SELECT COUNT(DISTINCT r.id)
		FROM requests r
		JOIN request_types rt ON rt.id = r.request_type_id
		LEFT JOIN request_students rs ON rs.request_id = r.id
		    AND rs.student_id = (SELECT id FROM student_profiles WHERE user_id = ? LIMIT 1)
		WHERE rt.code = ? AND r.deleted_at IS NULL
		  AND (r.requester_user_id = ? OR rs.request_id IS NOT NULL)
	`, userID, typeKey, userID).Scan(&totalItems)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(`
		SELECT r.id, ANY_VALUE(rt.label) AS label, r.reason, r.status, r.request_date, r.created_at, r.updated_at, r.start_time, r.end_time,
		       ANY_VALUE(COALESCE(sp_req.full_name, sp_target.full_name, '')) AS student_name,
		       ANY_VALUE(COALESCE(c_req.class_name, c_target.class_name, '-')) AS class_name,
		       ANY_VALUE(COALESCE(sp_req.student_code, sp_target.student_code, '-')) AS student_code,
		       ANY_VALUE(COALESCE(u.email, '-')) AS email
		FROM requests r
		JOIN request_types rt ON rt.id = r.request_type_id
		JOIN users u ON u.id = r.requester_user_id
		LEFT JOIN student_profiles sp_req ON sp_req.user_id = r.requester_user_id
		LEFT JOIN student_class_enrollments sce_req ON sce_req.student_id = sp_req.id AND sce_req.is_active = 1
		LEFT JOIN classes c_req ON c_req.id = sce_req.class_id
		LEFT JOIN request_students rs ON rs.request_id = r.id
		    AND rs.student_id = (SELECT id FROM student_profiles WHERE user_id = ? LIMIT 1)
		LEFT JOIN student_profiles sp_target ON sp_target.id = rs.student_id
		LEFT JOIN student_class_enrollments sce_target ON sce_target.student_id = sp_target.id AND sce_target.is_active = 1
		LEFT JOIN classes c_target ON c_target.id = sce_target.class_id
		WHERE rt.code = ? AND r.deleted_at IS NULL
		  AND (r.requester_user_id = ? OR rs.request_id IS NOT NULL)
		GROUP BY r.id
		ORDER BY r.created_at DESC
		LIMIT ? OFFSET ?
	`, userID, typeKey, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanLetterRows(rows)
	if err != nil {
		return nil, err
	}
	if err := r.populateStudentsForLetters(items); err != nil {
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
		SELECT r.id, ANY_VALUE(rt.label) AS label, r.reason, r.status, r.request_date, r.created_at, r.updated_at, r.start_time, r.end_time,
		       ANY_VALUE(COALESCE(sp.full_name,'')) AS student_name,
		       ANY_VALUE(COALESCE(c.class_name,'-')) AS class_name,
		       ANY_VALUE(COALESCE(sp.student_code,'-')) AS student_code,
		       ANY_VALUE(COALESCE(u.email,'-')) AS email
		FROM requests r
		JOIN request_types rt ON rt.id = r.request_type_id
		JOIN users u ON u.id = r.requester_user_id
		LEFT JOIN student_profiles sp ON sp.user_id = u.id
		LEFT JOIN student_class_enrollments sce ON sce.student_id = sp.id AND sce.is_active = 1
		LEFT JOIN classes c ON c.id = sce.class_id
		WHERE rt.code = ? AND r.deleted_at IS NULL
		GROUP BY r.id
		ORDER BY r.created_at DESC
		LIMIT ? OFFSET ?`, typeKey, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanLetterRows(rows)
	if err != nil {
		return nil, err
	}
	if err := r.populateStudentsForLetters(items); err != nil {
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
			startTime, endTime                                               sql.NullString
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

func (r *letterRepository) populateStudentsForLetters(items []domain.LetterListItem) error {
	if len(items) == 0 {
		return nil
	}

	reqIDs := make([]int, len(items))
	reqIDMap := make(map[int]int)
	for i, item := range items {
		reqIDs[i] = item.ID
		reqIDMap[item.ID] = i
	}

	uniqueReqIDs := make([]int, 0, len(reqIDs))
	seenIDs := make(map[int]bool)
	for _, id := range reqIDs {
		if !seenIDs[id] {
			seenIDs[id] = true
			uniqueReqIDs = append(uniqueReqIDs, id)
		}
	}

	placeholders := make([]string, len(uniqueReqIDs))
	args := make([]any, len(uniqueReqIDs))
	for i, id := range uniqueReqIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT rs.request_id, sp.full_name,
		       COALESCE((
		           SELECT c.class_name FROM student_class_enrollments sce
		           JOIN classes c ON c.id = sce.class_id
		           WHERE sce.student_id = sp.id AND sce.is_active = 1
		           LIMIT 1
		       ), '-'),
		       COALESCE(sp.student_code, '-'), COALESCE(u.email, '-')
		FROM request_students rs
		JOIN student_profiles sp ON sp.id = rs.student_id
		JOIN users u ON u.id = sp.user_id
		WHERE rs.request_id IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	studentsMap := make(map[int][]domain.StudentInfoDTO)
	for rows.Next() {
		var reqID int
		var s domain.StudentInfoDTO
		if err := rows.Scan(&reqID, &s.Name, &s.Class, &s.NISN, &s.Email); err != nil {
			return err
		}
		studentsMap[reqID] = append(studentsMap[reqID], s)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for reqID, students := range studentsMap {
		if idx, ok := reqIDMap[reqID]; ok {
			items[idx].Students = students
			if len(students) > 0 {
				items[idx].StudentInfo = students[0]
			}
		}
	}

	return nil
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
	case "rejected":
		return "ditolak"
	case "cancelled":
		return "dibatalkan"
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

func formatNullableTime(t sql.NullString) string {
	if !t.Valid {
		return "-"
	}
	return t.String
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
			SELECT r.id, ANY_VALUE(rt.label) AS label, r.reason, r.status, r.request_date, r.created_at, r.updated_at, r.start_time, r.end_time,
			       ANY_VALUE(COALESCE(sp.full_name,'')) AS student_name,
			       ANY_VALUE(COALESCE(c.class_name,'-')) AS class_name,
			       ANY_VALUE(COALESCE(sp.student_code,'-')) AS student_code,
			       ANY_VALUE(COALESCE(u.email,'-')) AS email
			FROM requests r
			JOIN request_types rt ON rt.id = r.request_type_id
			JOIN users u ON u.id = r.requester_user_id
			JOIN request_students rs ON rs.request_id = r.id
			JOIN student_profiles sp ON sp.id = rs.student_id
			JOIN student_class_enrollments sce ON sce.student_id = sp.id AND sce.is_active = 1
			LEFT JOIN classes c ON c.id = sce.class_id
			WHERE rt.code = 'dispensasi' AND r.deleted_at IS NULL AND (%s)
			GROUP BY r.id
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
			SELECT r.id, ANY_VALUE(rt.label) AS label, r.reason, r.status, r.request_date, r.created_at, r.updated_at, r.start_time, r.end_time,
			       ANY_VALUE(COALESCE(sp.full_name,'')) AS student_name,
			       ANY_VALUE(COALESCE(c.class_name,'-')) AS class_name,
			       ANY_VALUE(COALESCE(sp.student_code,'-')) AS student_code,
			       ANY_VALUE(COALESCE(u.email,'-')) AS email
			FROM requests r
			JOIN request_types rt ON rt.id = r.request_type_id
			JOIN users u ON u.id = r.requester_user_id
			JOIN request_students rs ON rs.request_id = r.id
			JOIN student_profiles sp ON sp.id = rs.student_id
			JOIN student_class_enrollments sce ON sce.student_id = sp.id AND sce.is_active = 1
			LEFT JOIN classes c ON c.id = sce.class_id
			WHERE rt.code = 'dispensasi' AND r.deleted_at IS NULL
			GROUP BY r.id
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
	if err := r.populateStudentsForLetters(items); err != nil {
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

func (r *letterRepository) IsActivePrincipal(userID int) (bool, error) {
	var id int64
	err := r.db.QueryRow(`
		SELECT id FROM principal_profiles
		WHERE user_id = ? AND active = 1 AND deleted_at IS NULL
		LIMIT 1
	`, userID).Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
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
		JOIN request_students rs ON rs.request_id = r.id
		JOIN student_profiles sp ON sp.id = rs.student_id
		JOIN student_class_enrollments sce ON sce.student_id = sp.id AND sce.is_active = 1
		WHERE rt.code = ? AND r.deleted_at IS NULL AND (%s)
	`, scopeFilter)

	var totalItems int
	if err := r.db.QueryRow(countQuery, typeKey).Scan(&totalItems); err != nil {
		return nil, err
	}

	rows, err := r.db.Query(fmt.Sprintf(`
		SELECT r.id, ANY_VALUE(rt.label) AS label, r.reason, r.status, r.request_date, r.created_at, r.updated_at, r.start_time, r.end_time,
		       ANY_VALUE(COALESCE(sp.full_name,'')) AS student_name,
		       ANY_VALUE(COALESCE(c.class_name,'-')) AS class_name,
		       ANY_VALUE(COALESCE(sp.student_code,'-')) AS student_code,
		       ANY_VALUE(COALESCE(u.email,'-')) AS email
		FROM requests r
		JOIN request_types rt ON rt.id = r.request_type_id
		JOIN users u ON u.id = r.requester_user_id
		JOIN request_students rs ON rs.request_id = r.id
		JOIN student_profiles sp ON sp.id = rs.student_id
		JOIN student_class_enrollments sce ON sce.student_id = sp.id AND sce.is_active = 1
		LEFT JOIN classes c ON c.id = sce.class_id
		WHERE rt.code = ? AND r.deleted_at IS NULL AND (%s)
		GROUP BY r.id
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
	if err := r.populateStudentsForLetters(items); err != nil {
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
		SELECT v.request_id, v.request_type_label, v.reason, 'pending', v.submitted_at, v.submitted_at, v.submitted_at, v.start_time, v.end_time,
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
	if err := r.populateStudentsForLetters(items); err != nil {
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
		SELECT r.id, rt.label, r.reason, r.status, r.request_date, r.created_at, r.updated_at, r.start_time, r.end_time,
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
	if err := r.populateStudentsForLetters(items); err != nil {
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

func (r *letterRepository) GetUserRole(userID int) (string, error) {
	var role string
	err := r.db.QueryRow(`SELECT role FROM users WHERE id = ? AND deleted_at IS NULL`, userID).Scan(&role)
	if err != nil {
		return "", err
	}
	return role, nil
}

func (r *letterRepository) GetRequestTypeInfo(typeID int) (*domain.RequestTypeInfo, error) {
	info := &domain.RequestTypeInfo{}
	err := r.db.QueryRow(
		`SELECT id, code, label, letter_prefix, requester_role, duration_days, is_active FROM request_types WHERE id = ?`,
		typeID,
	).Scan(&info.ID, &info.Code, &info.Label, &info.LetterPrefix, &info.RequesterRole, &info.DurationDays, &info.IsActive)
	if err != nil {
		return nil, err
	}
	return info, nil
}

func (r *letterRepository) HasActiveRequest(userID int, requestTypeID int, requestDate string) (bool, error) {
	var count int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM requests
		 WHERE requester_user_id = ? AND request_type_id = ? AND request_date = ?
		 AND status IN ('pending', 'approved')
		 AND deleted_at IS NULL`,
		userID, requestTypeID, requestDate,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
