package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Refliqx/backend-eletter/internal/domain"
)

type PermissionRepository interface {
	domain.PermissionRepository
}

type permissionRepository struct {
	db         *sql.DB
	schoolCode string
}

func NewPermissionRepository(db *sql.DB, schoolCode string) PermissionRepository {
	return &permissionRepository{db: db, schoolCode: schoolCode}
}

func (r *permissionRepository) ListAll() ([]domain.PermissionRequest, error) {
	rows, err := r.db.Query(`SELECT id, request_type_id, request_number, requester_user_id, reason, request_date, start_time, end_time, status, current_step, created_at, updated_at FROM requests WHERE deleted_at IS NULL ORDER BY request_date DESC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPermissionRequests(rows)
}

func (r *permissionRepository) ListByUser(userID int) ([]domain.PermissionRequest, error) {
	rows, err := r.db.Query(`SELECT id, request_type_id, request_number, requester_user_id, reason, request_date, start_time, end_time, status, current_step, created_at, updated_at FROM requests WHERE requester_user_id = ? AND deleted_at IS NULL ORDER BY request_date DESC, created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPermissionRequests(rows)
}

func (r *permissionRepository) ListClasses() ([]domain.PermissionClass, error) {
	rows, err := r.db.Query(`SELECT id AS class_id, class_name FROM classes ORDER BY class_name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.PermissionClass, 0)
	for rows.Next() {
		var c domain.PermissionClass
		if err := rows.Scan(&c.ClassID, &c.ClassName); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *permissionRepository) ListMajors() ([]domain.PermissionMajor, error) {
	rows, err := r.db.Query(`SELECT id AS major_id, name AS major_name, code AS major_short FROM majors ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.PermissionMajor, 0)
	for rows.Next() {
		var m domain.PermissionMajor
		if err := rows.Scan(&m.MajorID, &m.MajorName, &m.MajorShort); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *permissionRepository) GetUserByNISN(nisn string) (*domain.User, error) {
	row := r.db.QueryRow(`
		SELECT u.id, u.username, u.email, u.role, u.status, u.password_hash,
		       sp.full_name,
		       sp.student_code,
		       NULL as employee_code,
		       sp.gender,
		       sp.phone as phone_number,
		       (SELECT sce.class_id FROM student_class_enrollments sce WHERE sce.student_id = sp.id AND sce.is_active = 1 LIMIT 1) as class_id,
		       false as can_request_dispensasi,
		       COALESCE(sp.active, 0) as profile_completed
		FROM users u
		JOIN student_profiles sp ON sp.user_id = u.id
		WHERE sp.student_code = ? AND u.status = 'active' AND u.deleted_at IS NULL
		LIMIT 1`, nisn)
	return scanUser(row)
}

func (r *permissionRepository) GetUserByID(userID int) (*domain.User, error) {
	row := r.db.QueryRow(`
		SELECT u.id, u.username, u.email, u.role, u.status, u.password_hash,
		       CASE WHEN u.role IN ('teacher','kepala_sekolah') THEN tp.full_name
		            WHEN u.role = 'student' THEN sp.full_name
		       END as full_name,
		       CASE WHEN u.role = 'student' THEN sp.student_code ELSE NULL END as student_code,
		       CASE WHEN u.role IN ('teacher','kepala_sekolah') THEN tp.employee_code ELSE NULL END as employee_code,
		       COALESCE(tp.gender, sp.gender, NULL) as gender,
		       COALESCE(tp.phone, sp.phone, NULL) as phone_number,
		       CASE WHEN u.role = 'student'
		            THEN (SELECT sce.class_id FROM student_class_enrollments sce
		                  JOIN student_profiles sp2 ON sce.student_id = sp2.id
		                  WHERE sp2.user_id = u.id AND sce.is_active = 1 LIMIT 1)
		       END as class_id,
		       CASE WHEN u.role IN ('teacher','kepala_sekolah') THEN true ELSE false END as can_request_dispensasi,
		       CASE WHEN u.role IN ('teacher','kepala_sekolah') THEN COALESCE(tp.active, 0)
		            WHEN u.role = 'student' THEN COALESCE(sp.active, 0)
		            ELSE false
		       END as profile_completed
		FROM users u
		LEFT JOIN teacher_profiles tp ON tp.user_id = u.id
		LEFT JOIN student_profiles sp ON sp.user_id = u.id
		WHERE u.id = ? AND u.deleted_at IS NULL LIMIT 1`, userID)
	return scanUser(row)
}

func (r *permissionRepository) Create(req domain.CreatePermissionRequest) (int, error) {
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
		`INSERT INTO requests (request_number, academic_year_id, request_type_id, requester_user_id, reason, start_time, end_time, request_date, status)
		 VALUES (?,?,?,?,?,?,?,?, 'pending')`,
		requestNumber, academicYearID, req.TypeID, req.IDSiswa, req.Description, req.StartDate, req.EndDate, req.RequestDate,
	)
	if err != nil {
		return 0, err
	}
	lid, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	id := int(lid)

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func (r *permissionRepository) Update(req domain.UpdatePermissionRequest) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	updates := []string{}
	args := []any{}
	if req.Description != nil {
		updates = append(updates, "reason = ?")
		args = append(args, *req.Description)
	}
	if req.StartTime != nil {
		updates = append(updates, "start_time = ?")
		args = append(args, *req.StartTime)
	}
	if req.EndTime != nil {
		updates = append(updates, "end_time = ?")
		args = append(args, *req.EndTime)
	}
	if req.Status != nil {
		updates = append(updates, "status = ?")
		args = append(args, *req.Status)
	}
	updates = append(updates, "updated_at = NOW()")
	if len(updates) == 1 {
		return nil
	}
	args = append(args, req.RequestID)
	query := fmt.Sprintf("UPDATE requests SET %s WHERE id = ?", strings.Join(updates, ", "))
	if _, err := tx.Exec(query, args...); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *permissionRepository) Delete(requestID int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE requests SET deleted_at = NOW() WHERE id = ?`, requestID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *permissionRepository) Approve(req domain.ApprovalRequest, approverID int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Validate approval step & role authority
	callerTeacherID, _, approverRole, isDelegated, delegatedFromID, err := ValidateApprovalStep(tx, req.RequestID, req.StageID, approverID)
	if err != nil {
		return err
	}

	// 2. Perform the update based on role and delegation
	var res sql.Result
	if isDelegated {
		res, err = tx.Exec(
			`UPDATE request_approvals
			 SET status = ?, notes = ?, signature_url = ?, acted_at = NOW(), updated_at = NOW(),
			     is_delegated = 1, delegated_from_id = ?, approver_teacher_id = ?
			 WHERE request_id = ? AND step_no = ? AND deleted_at IS NULL`,
			req.Status, req.Notes, req.SignatureURL, delegatedFromID, callerTeacherID, req.RequestID, req.StageID,
		)
	} else if approverRole == "tatib" {
		res, err = tx.Exec(
			`UPDATE request_approvals
			 SET status = ?, notes = ?, signature_url = ?, acted_at = NOW(), updated_at = NOW(),
			     approver_teacher_id = ?
			 WHERE request_id = ? AND step_no = ? AND deleted_at IS NULL`,
			req.Status, req.Notes, req.SignatureURL, callerTeacherID, req.RequestID, req.StageID,
		)
	} else {
		res, err = tx.Exec(
			`UPDATE request_approvals
			 SET status = ?, notes = ?, signature_url = ?, acted_at = NOW(), updated_at = NOW()
			 WHERE request_id = ? AND step_no = ? AND deleted_at IS NULL`,
			req.Status, req.Notes, req.SignatureURL, req.RequestID, req.StageID,
		)
	}
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("approval step not found for this request")
	}

	// 3. Determine request status: stays 'pending' unless all required steps done or one rejected
	targetStatus := "pending"
	if req.Status == "rejected" {
		targetStatus = "rejected"
	} else if req.Status == "approved" || req.Status == "skipped" {
		var pendingCount int
		if err := tx.QueryRow(`
			SELECT COUNT(*) FROM request_approvals
			WHERE request_id = ? AND status = 'pending' AND deleted_at IS NULL`,
			req.RequestID,
		).Scan(&pendingCount); err != nil {
			return err
		}
		if pendingCount == 0 {
			targetStatus = "approved"
		}
	}

	// Calculate and update current_step if this step is successfully approved
	var currentStep int
	if req.Status == "approved" || req.Status == "skipped" {
		currentStep = req.StageID
	} else {
		currentStep = req.StageID - 1
		if currentStep < 0 {
			currentStep = 0
		}
	}

	if _, err := tx.Exec(`UPDATE requests SET status = ?, current_step = ?, updated_at = NOW() WHERE id = ?`, targetStatus, currentStep, req.RequestID); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *permissionRepository) ListRegistrationTokens() ([]domain.TokenRecord, error) {
	rows, err := r.db.Query(`SELECT token_id, 0 AS user_id, token, expires_at, used_count, usage_limit FROM registration_tokens ORDER BY token_id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.TokenRecord, 0)
	for rows.Next() {
		var rec domain.TokenRecord
		var usageLimit sql.NullInt64
		var expiresAt sql.NullTime
		if err := rows.Scan(&rec.TokenID, &rec.UserID, &rec.TokenHash, &expiresAt, &rec.UsedCount, &usageLimit); err != nil {
			return nil, err
		}
		if expiresAt.Valid {
			rec.ExpiresAt = &expiresAt.Time
		}
		if usageLimit.Valid {
			v := int(usageLimit.Int64)
			rec.UsageLimit = &v
		}
		rec.TokenType = "registration"
		rec.IsRevoked = false
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (r *permissionRepository) CreateOrUpdateRegistrationToken(token string, roleID int, usageLimit *int, expiresAt *time.Time) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO registration_tokens (token, role_id, usage_limit, used_count, expires_at)
		 VALUES (?, ?, ?, 0, ?)
		 ON DUPLICATE KEY UPDATE usage_limit = VALUES(usage_limit), expires_at = VALUES(expires_at)`,
		token, roleID, usageLimit, expiresAt,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *permissionRepository) GetRegistrationTokenByValue(token string) (*domain.TokenRecord, error) {
	row := r.db.QueryRow(`SELECT token_id, 0 AS user_id, token, expires_at, used_count, usage_limit FROM registration_tokens WHERE token = ? AND used_count < usage_limit AND (expires_at IS NULL OR expires_at > NOW()) LIMIT 1`, token)
	var rec domain.TokenRecord
	var usageLimit sql.NullInt64
	var expiresAt sql.NullTime
	if err := row.Scan(&rec.TokenID, &rec.UserID, &rec.TokenHash, &expiresAt, &rec.UsedCount, &usageLimit); err != nil {
		return nil, err
	}
	if expiresAt.Valid {
		rec.ExpiresAt = &expiresAt.Time
	}
	if usageLimit.Valid {
		v := int(usageLimit.Int64)
		rec.UsageLimit = &v
	}
	rec.TokenType = "registration"
	rec.IsRevoked = false
	return &rec, nil
}

func scanPermissionRequests(rows *sql.Rows) ([]domain.PermissionRequest, error) {
	out := make([]domain.PermissionRequest, 0)
	for rows.Next() {
		var req domain.PermissionRequest
		if err := rows.Scan(
			&req.RequestID,
			&req.TypeID,
			&req.RequestNumber,
			&req.RequesterUserID,
			&req.Reason,
			&req.RequestDate,
			&req.StartTime,
			&req.EndTime,
			&req.Status,
			&req.CurrentStep,
			&req.CreatedAt,
			&req.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	return out, rows.Err()
}

func (r *permissionRepository) CancelRequest(requestID, userID int, reason string) error {
	// Verify ownership and status
	var requesterID int
	var status string
	err := r.db.QueryRow(`SELECT requester_user_id, status FROM requests WHERE id = ?`, requestID).Scan(&requesterID, &status)
	if err != nil {
		return errors.New("Permintaan tidak ditemukan")
	}
	if requesterID != userID {
		return errors.New("Anda tidak memiliki izin untuk membatalkan permintaan ini")
	}
	if status != "pending" {
		return errors.New("Hanya permintaan dengan status pending yang dapat dibatalkan")
	}

	_, err = r.db.Exec(`UPDATE requests SET status = 'cancelled', cancelled_at = NOW(), cancelled_by = ?, cancel_reason = ? WHERE id = ?`,
		userID, reason, requestID)
	return err
}

func (r *permissionRepository) GetRequestDetail(requestID int) (any, error) {
	// Get request info
	var req struct {
		ID            int     `json:"id"`
		RequestNumber string  `json:"request_number"`
		TypeID        int     `json:"type_id"`
		Status        string  `json:"status"`
		Reason        *string `json:"reason"`
		CurrentStep   int     `json:"current_step"`
		CreatedAt     string  `json:"created_at"`
	}
	err := r.db.QueryRow(`SELECT id, request_number, request_type_id, status, reason, current_step, created_at FROM requests WHERE id = ?`, requestID).
		Scan(&req.ID, &req.RequestNumber, &req.TypeID, &req.Status, &req.Reason, &req.CurrentStep, &req.CreatedAt)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(`
		SELECT ra.id, ra.step_no, ra.approver_role, ra.status, ra.notes, ra.signature_url, ra.acted_at,
		       COALESCE(tp.full_name, pp.full_name, '') as approver_name
		FROM request_approvals ra
		LEFT JOIN teacher_profiles tp ON tp.id = ra.approver_teacher_id
		LEFT JOIN principal_profiles pp ON pp.id = ra.approver_principal_id
		WHERE ra.request_id = ? AND ra.deleted_at IS NULL
		ORDER BY ra.step_no
	`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type ApprovalStep struct {
		ID           int     `json:"id"`
		StepNo       int     `json:"step_no"`
		ApproverRole string  `json:"approver_role"`
		Status       string  `json:"status"`
		Notes        *string `json:"notes"`
		SignatureURL *string `json:"signature_url"`
		ActedAt      *string `json:"acted_at"`
		ApproverName string  `json:"approver_name"`
	}

	var steps []ApprovalStep
	for rows.Next() {
		var s ApprovalStep
		if err := rows.Scan(&s.ID, &s.StepNo, &s.ApproverRole, &s.Status, &s.Notes, &s.SignatureURL, &s.ActedAt, &s.ApproverName); err != nil {
			continue
		}
		steps = append(steps, s)
	}

	return map[string]any{"request": req, "approval_steps": steps}, nil
}

func (r *permissionRepository) GetTeacherRoles(userID int) (any, error) {
	rows, err := r.db.Query(`
		SELECT tr.id, tr.role_name, tr.status, tr.verified_at
		FROM teacher_roles tr
		JOIN teacher_profiles tp ON tp.id = tr.teacher_id
		WHERE tp.user_id = ?
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type Role struct {
		ID         int     `json:"id"`
		RoleName   string  `json:"role_name"`
		Status     string  `json:"status"`
		VerifiedAt *string `json:"verified_at"`
	}
	var roles []Role
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role.ID, &role.RoleName, &role.Status, &role.VerifiedAt); err != nil {
			continue
		}
		roles = append(roles, role)
	}
	return roles, nil
}

func (r *permissionRepository) RequestTeacherRole(userID int, roleName string, meta domain.TeacherRoleMetadata) error {
	var teacherID int
	err := r.db.QueryRow(`SELECT id FROM teacher_profiles WHERE user_id = ?`, userID).Scan(&teacherID)
	if err != nil {
		return errors.New("Profil guru tidak ditemukan")
	}

	var academicYearID int
	_ = r.db.QueryRow(`SELECT id FROM academic_years WHERE is_active = 1 LIMIT 1`).Scan(&academicYearID)

	// Build subject_ids as comma-separated string for storage in the staging column.
	var subjectIDsStr *string
	if len(meta.SubjectIDs) > 0 {
		parts := make([]string, len(meta.SubjectIDs))
		for i, id := range meta.SubjectIDs {
			parts[i] = strconv.Itoa(id)
		}
		s := strings.Join(parts, ",")
		subjectIDsStr = &s
	}

	_, err = r.db.Exec(
		`INSERT INTO teacher_roles
			(teacher_id, role_name, academic_year_id, status, homeroom_class_id, major_id, subject_ids)
		 VALUES (?, ?, ?, 'pending', ?, ?, ?)`,
		teacherID, roleName, academicYearID,
		meta.HomeroomClassID, meta.MajorID, subjectIDsStr,
	)
	return err
}

func (r *permissionRepository) ListPendingTeacherRoles(status string) ([]domain.PendingTeacherRole, error) {
	if status == "" {
		status = "pending"
	}
	rows, err := r.db.Query(`
		SELECT
			tr.id,
			tp.id AS teacher_id,
			u.id  AS teacher_user_id,
			tp.full_name,
			tr.role_name,
			tr.status,
			tr.homeroom_class_id,
			c.class_name,
			tr.major_id,
			m.name AS major_name,
			tr.subject_ids,
			DATE_FORMAT(tr.created_at, '%Y-%m-%dT%H:%i:%sZ') AS created_at
		FROM teacher_roles tr
		JOIN teacher_profiles tp ON tp.id = tr.teacher_id
		JOIN users u ON u.id = tp.user_id
		LEFT JOIN classes c ON c.id = tr.homeroom_class_id
		LEFT JOIN majors m ON m.id = tr.major_id
		WHERE tr.status = ?
		ORDER BY tr.created_at DESC
	`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.PendingTeacherRole
	for rows.Next() {
		var rec domain.PendingTeacherRole
		var className sql.NullString
		var majorName sql.NullString
		if err := rows.Scan(
			&rec.ID, &rec.TeacherID, &rec.TeacherUserID, &rec.TeacherName,
			&rec.RoleName, &rec.Status,
			&rec.HomeroomClassID, &className,
			&rec.MajorID, &majorName,
			&rec.SubjectIDs,
			&rec.CreatedAt,
		); err != nil {
			return nil, err
		}
		if className.Valid {
			rec.HomeroomClass = &className.String
		}
		if majorName.Valid {
			rec.MajorName = &majorName.String
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (r *permissionRepository) RejectTeacherRole(id, adminUserID int, _ string) error {
	_, err := r.db.Exec(
		`UPDATE teacher_roles SET status = 'rejected', verified_by = ?, verified_at = NOW(), updated_at = NOW() WHERE id = ?`,
		adminUserID, id,
	)
	return err
}

func (r *permissionRepository) CreateDelegation(userID, delegateUserID int, validFrom, validUntil, reason string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var originalTeacherID int64
	err = tx.QueryRow(`SELECT id FROM teacher_profiles WHERE user_id = ? AND active = 1 AND deleted_at IS NULL`, userID).Scan(&originalTeacherID)
	if err != nil {
		return fmt.Errorf("profil guru pemberi delegasi tidak aktif: %w", err)
	}

	var delegateTeacherID int64
	err = tx.QueryRow(`SELECT id FROM teacher_profiles WHERE user_id = ? AND active = 1 AND deleted_at IS NULL`, delegateUserID).Scan(&delegateTeacherID)
	if err != nil {
		return fmt.Errorf("profil guru penerima delegasi tidak aktif: %w", err)
	}

	// Fetch active roles for the original teacher
	rows, err := tx.Query(`SELECT role_name FROM teacher_roles WHERE teacher_id = ? AND status = 'active'`, originalTeacherID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err == nil {
			roles = append(roles, role)
		}
	}

	if len(roles) == 0 {
		return fmt.Errorf("guru pemberi delegasi tidak memiliki peran aktif yang dapat didelegasikan")
	}

	for _, role := range roles {
		_, err = tx.Exec(`
			INSERT INTO request_approval_delegates (original_teacher_id, delegate_teacher_id, delegate_role, valid_from, valid_until, reason, created_by_user_id, is_active)
			VALUES (?, ?, ?, ?, ?, ?, ?, 1)
		`, originalTeacherID, delegateTeacherID, role, validFrom, validUntil, reason, userID)
		if err != nil {
			return fmt.Errorf("gagal membuat delegasi untuk peran %s: %w", role, err)
		}
	}

	return tx.Commit()
}

func (r *permissionRepository) ListDelegations(userID int) (any, error) {
	rows, err := r.db.Query(`
		SELECT rad.id, tp_del.user_id as delegate_user_id, rad.valid_from, rad.valid_until, rad.reason,
		       COALESCE(tp_del.full_name, '') as delegate_name
		FROM request_approval_delegates rad
		JOIN teacher_profiles tp_orig ON tp_orig.id = rad.original_teacher_id
		JOIN teacher_profiles tp_del ON tp_del.id = rad.delegate_teacher_id
		WHERE tp_orig.user_id = ? AND rad.valid_until >= NOW() AND rad.is_active = 1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type Delegation struct {
		ID             int    `json:"id"`
		DelegateUserID int    `json:"delegate_user_id"`
		ValidFrom      string `json:"valid_from"`
		ValidUntil     string `json:"valid_until"`
		Reason         string `json:"reason"`
		DelegateName   string `json:"delegate_name"`
	}
	var delegations []Delegation
	for rows.Next() {
		var d Delegation
		if err := rows.Scan(&d.ID, &d.DelegateUserID, &d.ValidFrom, &d.ValidUntil, &d.Reason, &d.DelegateName); err != nil {
			continue
		}
		delegations = append(delegations, d)
	}
	return delegations, nil
}

func (r *permissionRepository) DeleteDelegation(id, userID int) error {
	var originalTeacherID int64
	err := r.db.QueryRow(`SELECT id FROM teacher_profiles WHERE user_id = ? AND active = 1 AND deleted_at IS NULL`, userID).Scan(&originalTeacherID)
	if err != nil {
		return fmt.Errorf("profil guru pemberi delegasi tidak aktif: %w", err)
	}

	res, err := r.db.Exec(`UPDATE request_approval_delegates SET is_active = 0 WHERE id = ? AND original_teacher_id = ?`, id, originalTeacherID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return errors.New("delegasi tidak ditemukan atau tidak berwenang menghapus")
	}
	return nil
}
