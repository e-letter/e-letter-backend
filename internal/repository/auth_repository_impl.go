package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Refliqx/backend-eletter/internal/domain"
	"github.com/go-sql-driver/mysql"
)

func normalizeEmail(email string) []string {
	email = strings.ToLower(strings.TrimSpace(email))
	if !strings.Contains(email, "@") {
		return []string{email}
	}
	parts := strings.Split(email, "@")
	prefix := parts[0]
	if strings.HasSuffix(email, "@smkn2singosari.sch.id") {
		return []string{
			email,
			prefix + "@guru.smk.belajar.id",
			prefix + "@smk.belajar.id",
		}
	}
	if strings.HasSuffix(email, "@guru.smk.belajar.id") {
		return []string{
			email,
			prefix + "@smkn2singosari.sch.id",
		}
	}
	if strings.HasSuffix(email, "@smk.belajar.id") {
		return []string{
			email,
			prefix + "@smkn2singosari.sch.id",
		}
	}
	return []string{email}
}

func (r *authRepository) GetUserByLoginIdentifiers(id string) (*domain.User, error) {
	emails := normalizeEmail(id)
	var email1, email2, email3 string
	email1 = id
	if len(emails) > 0 {
		email1 = emails[0]
	}
	email2 = email1
	if len(emails) > 1 {
		email2 = emails[1]
	}
	email3 = email1
	if len(emails) > 2 {
		email3 = emails[2]
	}

	query := `
		SELECT u.id, u.username, u.email, u.role, u.status, u.password_hash,
		       CASE WHEN u.role = 'teacher' THEN tp.full_name
		            WHEN u.role = 'kepala_sekolah' THEN pp.full_name
		            WHEN u.role = 'student' THEN sp.full_name
		       END as full_name,
		       CASE WHEN u.role = 'student' THEN sp.student_code ELSE NULL END as student_code,
		       CASE WHEN u.role = 'teacher' THEN tp.employee_code
		            WHEN u.role = 'kepala_sekolah' THEN pp.employee_code
		            ELSE NULL
		       END as employee_code,
		       COALESCE(tp.gender, sp.gender, pp.gender, NULL) as gender,
		       COALESCE(tp.phone, sp.phone, pp.phone, NULL) as phone_number,
		       CASE WHEN u.role = 'student'
		            THEN (SELECT class_id FROM student_class_enrollments sce WHERE sce.student_id = sp.id AND sce.is_active = 1 LIMIT 1)
		            WHEN u.role = 'teacher'
		            THEN (SELECT class_id FROM class_homeroom_assignments cha WHERE cha.teacher_id = tp.id AND cha.is_active = 1 LIMIT 1)
		       END as class_id,
		       CASE WHEN u.role IN ('teacher','kepala_sekolah') THEN true ELSE false END as can_request_dispensasi,
		       CASE WHEN u.role = 'teacher' THEN
		                 CASE WHEN COALESCE(tp.active, 0) = 1
		                           AND tp.employee_code IS NOT NULL AND tp.employee_code != ''
		                           AND tp.signature_url IS NOT NULL AND tp.signature_url != ''
		                      THEN true ELSE false END
		            WHEN u.role = 'kepala_sekolah' THEN
		                 CASE WHEN COALESCE(pp.active, 0) = 1
		                           AND pp.employee_code IS NOT NULL AND pp.employee_code != ''
		                           AND pp.signature_url IS NOT NULL AND pp.signature_url != ''
		                      THEN true ELSE false END
		            WHEN u.role = 'student' THEN
		                 CASE WHEN COALESCE(sp.active, 0) = 1
		                           AND sp.student_code IS NOT NULL AND sp.student_code != ''
		                           AND sp.signature_url IS NOT NULL AND sp.signature_url != ''
		                      THEN true ELSE false END
		            ELSE false
		       END as profile_completed,
		       COALESCE(tp.created_at, sp.created_at, pp.created_at, u.created_at) as created_at,
		       COALESCE(tp.updated_at, sp.updated_at, pp.updated_at, u.updated_at) as updated_at,
		       COALESCE(tp.signature_url, sp.signature_url, pp.signature_url, NULL) as signature_url
		FROM users u
		LEFT JOIN teacher_profiles tp ON tp.user_id = u.id
		LEFT JOIN student_profiles sp ON sp.user_id = u.id
		LEFT JOIN principal_profiles pp ON pp.user_id = u.id
		WHERE (u.username = ? OR u.email = ? OR u.email = ? OR u.email = ? OR sp.student_code = ? OR tp.employee_code = ? OR pp.employee_code = ?)
		  AND u.status IN ('active', 'pending', 'inactive', 'blocked') AND u.deleted_at IS NULL
		LIMIT 1
	`
	row := r.db.QueryRow(query, id, email1, email2, email3, id, id, id)
	return scanUser(row)
}

func (r *authRepository) GetUserByEmail(email string) (*domain.User, error) {
	emails := normalizeEmail(email)
	var email1, email2, email3 string
	email1 = email
	if len(emails) > 0 {
		email1 = emails[0]
	}
	email2 = email1
	if len(emails) > 1 {
		email2 = emails[1]
	}
	email3 = email1
	if len(emails) > 2 {
		email3 = emails[2]
	}

	query := `
		SELECT u.id, u.username, u.email, u.role, u.status, u.password_hash,
		       CASE WHEN u.role = 'teacher' THEN tp.full_name
		            WHEN u.role = 'kepala_sekolah' THEN pp.full_name
		            WHEN u.role = 'student' THEN sp.full_name
		       END as full_name,
		       CASE WHEN u.role = 'student' THEN sp.student_code ELSE NULL END as student_code,
		       CASE WHEN u.role = 'teacher' THEN tp.employee_code
		            WHEN u.role = 'kepala_sekolah' THEN pp.employee_code
		            ELSE NULL
		       END as employee_code,
		       COALESCE(tp.gender, sp.gender, pp.gender, NULL) as gender,
		       COALESCE(tp.phone, sp.phone, pp.phone, NULL) as phone_number,
		       CASE WHEN u.role = 'student'
		            THEN (SELECT class_id FROM student_class_enrollments sce WHERE sce.student_id = sp.id AND sce.is_active = 1 LIMIT 1)
		            WHEN u.role = 'teacher'
		            THEN (SELECT class_id FROM class_homeroom_assignments cha WHERE cha.teacher_id = tp.id AND cha.is_active = 1 LIMIT 1)
		       END as class_id,
		       CASE WHEN u.role IN ('teacher','kepala_sekolah') THEN true ELSE false END as can_request_dispensasi,
		       CASE WHEN u.role = 'teacher' THEN
		                 CASE WHEN COALESCE(tp.active, 0) = 1
		                           AND tp.employee_code IS NOT NULL AND tp.employee_code != ''
		                           AND tp.signature_url IS NOT NULL AND tp.signature_url != ''
		                      THEN true ELSE false END
		            WHEN u.role = 'kepala_sekolah' THEN
		                 CASE WHEN COALESCE(pp.active, 0) = 1
		                           AND pp.employee_code IS NOT NULL AND pp.employee_code != ''
		                           AND pp.signature_url IS NOT NULL AND pp.signature_url != ''
		                      THEN true ELSE false END
		            WHEN u.role = 'student' THEN
		                 CASE WHEN COALESCE(sp.active, 0) = 1
		                           AND sp.student_code IS NOT NULL AND sp.student_code != ''
		                           AND sp.signature_url IS NOT NULL AND sp.signature_url != ''
		                      THEN true ELSE false END
		            ELSE false
		       END as profile_completed,
		       COALESCE(tp.created_at, sp.created_at, pp.created_at, u.created_at) as created_at,
		       COALESCE(tp.updated_at, sp.updated_at, pp.updated_at, u.updated_at) as updated_at,
		       COALESCE(tp.signature_url, sp.signature_url, pp.signature_url, NULL) as signature_url
		FROM users u
		LEFT JOIN teacher_profiles tp ON tp.user_id = u.id
		LEFT JOIN student_profiles sp ON sp.user_id = u.id
		LEFT JOIN principal_profiles pp ON pp.user_id = u.id
		WHERE (u.email = ? OR u.email = ? OR u.email = ?) AND u.status = 'active' AND u.deleted_at IS NULL
		LIMIT 1
	`
	var user domain.User
	if err := r.db.QueryRow(query, email1, email2, email3).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.Role,
		&user.Status,
		&user.PasswordHash,
		&user.FullName,
		&user.StudentCode,
		&user.EmployeeCode,
		&user.Gender,
		&user.PhoneNumber,
		&user.ClassID,
		&user.CanRequestDispensasi,
		&user.ProfileCompleted,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.SignatureURL,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

// GetUserByEmailAnyStatus looks up a user by email regardless of their account status.
// This is intentionally used during registration to block duplicate email registrations
// even when an existing account is still in 'pending' state.
func (r *authRepository) GetUserByEmailAnyStatus(email string) (*domain.User, error) {
	emails := normalizeEmail(email)
	var email1, email2, email3 string
	email1 = email
	if len(emails) > 0 {
		email1 = emails[0]
	}
	email2 = email1
	if len(emails) > 1 {
		email2 = emails[1]
	}
	email3 = email1
	if len(emails) > 2 {
		email3 = emails[2]
	}

	query := `
		SELECT u.id, u.username, u.email, u.role, u.status, u.password_hash,
		       CASE WHEN u.role = 'teacher' THEN tp.full_name
		            WHEN u.role = 'kepala_sekolah' THEN pp.full_name
		            WHEN u.role = 'student' THEN sp.full_name
		       END as full_name,
		       CASE WHEN u.role = 'student' THEN sp.student_code ELSE NULL END as student_code,
		       CASE WHEN u.role = 'teacher' THEN tp.employee_code
		            WHEN u.role = 'kepala_sekolah' THEN pp.employee_code
		            ELSE NULL
		       END as employee_code,
		       COALESCE(tp.gender, sp.gender, pp.gender, NULL) as gender,
		       COALESCE(tp.phone, sp.phone, pp.phone, NULL) as phone_number,
		       CASE WHEN u.role = 'student'
		            THEN (SELECT class_id FROM student_class_enrollments sce WHERE sce.student_id = sp.id AND sce.is_active = 1 LIMIT 1)
		            WHEN u.role = 'teacher'
		            THEN (SELECT class_id FROM class_homeroom_assignments cha WHERE cha.teacher_id = tp.id AND cha.is_active = 1 LIMIT 1)
		       END as class_id,
		       CASE WHEN u.role IN ('teacher','kepala_sekolah') THEN true ELSE false END as can_request_dispensasi,
		       CASE WHEN u.role = 'teacher' THEN
		                 CASE WHEN COALESCE(tp.active, 0) = 1
		                           AND tp.employee_code IS NOT NULL AND tp.employee_code != ''
		                           AND tp.signature_url IS NOT NULL AND tp.signature_url != ''
		                      THEN true ELSE false END
		            WHEN u.role = 'kepala_sekolah' THEN
		                 CASE WHEN COALESCE(pp.active, 0) = 1
		                           AND pp.employee_code IS NOT NULL AND pp.employee_code != ''
		                           AND pp.signature_url IS NOT NULL AND pp.signature_url != ''
		                      THEN true ELSE false END
		            WHEN u.role = 'student' THEN
		                 CASE WHEN COALESCE(sp.active, 0) = 1
		                           AND sp.student_code IS NOT NULL AND sp.student_code != ''
		                           AND sp.signature_url IS NOT NULL AND sp.signature_url != ''
		                      THEN true ELSE false END
		            ELSE false
		       END as profile_completed,
		       COALESCE(tp.created_at, sp.created_at, pp.created_at, u.created_at) as created_at,
		       COALESCE(tp.updated_at, sp.updated_at, pp.updated_at, u.updated_at) as updated_at,
		       COALESCE(tp.signature_url, sp.signature_url, pp.signature_url, NULL) as signature_url
		FROM users u
		LEFT JOIN teacher_profiles tp ON tp.user_id = u.id
		LEFT JOIN student_profiles sp ON sp.user_id = u.id
		LEFT JOIN principal_profiles pp ON pp.user_id = u.id
		WHERE (u.email = ? OR u.email = ? OR u.email = ?) AND u.deleted_at IS NULL
		LIMIT 1
	`
	var user domain.User
	if err := r.db.QueryRow(query, email1, email2, email3).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.Role,
		&user.Status,
		&user.PasswordHash,
		&user.FullName,
		&user.StudentCode,
		&user.EmployeeCode,
		&user.Gender,
		&user.PhoneNumber,
		&user.ClassID,
		&user.CanRequestDispensasi,
		&user.ProfileCompleted,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.SignatureURL,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *authRepository) CreateUser(roleID int, email, passwordHash, status string) (int, error) {
	role := "student"
	if roleID == 2 {
		role = "teacher"
	}
	if err := ValidateRefValue(r.db, "user_status", status); err != nil {
		return 0, err
	}
	res, err := r.db.Exec(
		`INSERT INTO users (email, password_hash, role, status) VALUES (?, ?, ?, ?)`,
		email, passwordHash, role, status,
	)
	if err != nil {
		// MySQL error 1062 = Duplicate entry — the UNIQUE KEY `uq_email` was violated.
		// This can happen via a TOCTOU race when two concurrent registrations both pass
		// the app-level GetUserByEmailAnyStatus check before either INSERT commits.
		// Return a "terdaftar" error so auth_handler.go maps it to HTTP 409 Conflict.
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return 0, fmt.Errorf("Email sudah terdaftar")
		}
		return 0, err
	}
	id, err := res.LastInsertId()
	return int(id), err
}

func (r *authRepository) UpdateUserProfile(userID int, fullName string, profileCompleted bool) error {
	var role string
	err := r.db.QueryRow(`SELECT role FROM users WHERE id = ?`, userID).Scan(&role)
	if err != nil {
		return err
	}

	if role == "student" {
		_, err = r.db.Exec(
			`INSERT INTO student_profiles (user_id, full_name, active) VALUES (?, ?, ?)
			 ON DUPLICATE KEY UPDATE full_name = VALUES(full_name), active = VALUES(active)`,
			userID, fullName, profileCompleted,
		)
	} else if role == "kepala_sekolah" {
		if profileCompleted {
			if err := ValidateNoActivePrincipal(r.db, 0); err != nil {
				return err
			}
		}
		_, err = r.db.Exec(
			`INSERT INTO principal_profiles (user_id, full_name, active) VALUES (?, ?, ?)
			 ON DUPLICATE KEY UPDATE full_name = VALUES(full_name), active = VALUES(active)`,
			userID, fullName, profileCompleted,
		)
	} else {
		_, err = r.db.Exec(
			`INSERT INTO teacher_profiles (user_id, full_name, active) VALUES (?, ?, ?)
			 ON DUPLICATE KEY UPDATE full_name = VALUES(full_name), active = VALUES(active)`,
			userID, fullName, profileCompleted,
		)
	}
	return err
}

func (r *authRepository) GetUserByID(userID int) (*domain.User, error) {
	query := `
		SELECT u.id, u.username, u.email, u.role, u.status, u.password_hash,
		       CASE WHEN u.role = 'teacher' THEN tp.full_name
		            WHEN u.role = 'kepala_sekolah' THEN pp.full_name
		            WHEN u.role = 'student' THEN sp.full_name
		       END as full_name,
		       CASE WHEN u.role = 'student' THEN sp.student_code ELSE NULL END as student_code,
		       CASE WHEN u.role = 'teacher' THEN tp.employee_code
		            WHEN u.role = 'kepala_sekolah' THEN pp.employee_code
		            ELSE NULL
		       END as employee_code,
		       COALESCE(tp.gender, sp.gender, pp.gender, NULL) as gender,
		       COALESCE(tp.phone, sp.phone, pp.phone, NULL) as phone_number,
		       CASE WHEN u.role = 'student'
		            THEN (SELECT class_id FROM student_class_enrollments sce WHERE sce.student_id = sp.id AND sce.is_active = 1 LIMIT 1)
		            WHEN u.role = 'teacher'
		            THEN (SELECT class_id FROM class_homeroom_assignments cha WHERE cha.teacher_id = tp.id AND cha.is_active = 1 LIMIT 1)
		       END as class_id,
		       CASE WHEN u.role IN ('teacher','kepala_sekolah') THEN true ELSE false END as can_request_dispensasi,
		       CASE WHEN u.role = 'teacher' THEN
		                 CASE WHEN COALESCE(tp.active, 0) = 1
		                           AND tp.employee_code IS NOT NULL AND tp.employee_code != ''
		                           AND tp.signature_url IS NOT NULL AND tp.signature_url != ''
		                      THEN true ELSE false END
		            WHEN u.role = 'kepala_sekolah' THEN
		                 CASE WHEN COALESCE(pp.active, 0) = 1
		                           AND pp.employee_code IS NOT NULL AND pp.employee_code != ''
		                           AND pp.signature_url IS NOT NULL AND pp.signature_url != ''
		                      THEN true ELSE false END
		            WHEN u.role = 'student' THEN
		                 CASE WHEN COALESCE(sp.active, 0) = 1
		                           AND sp.student_code IS NOT NULL AND sp.student_code != ''
		                           AND sp.signature_url IS NOT NULL AND sp.signature_url != ''
		                      THEN true ELSE false END
		            ELSE false
		       END as profile_completed,
		       COALESCE(tp.created_at, sp.created_at, pp.created_at, u.created_at) as created_at,
		       COALESCE(tp.updated_at, sp.updated_at, pp.updated_at, u.updated_at) as updated_at,
		       COALESCE(tp.signature_url, sp.signature_url, pp.signature_url, NULL) as signature_url
		FROM users u
		LEFT JOIN teacher_profiles tp ON tp.user_id = u.id
		LEFT JOIN student_profiles sp ON sp.user_id = u.id
		LEFT JOIN principal_profiles pp ON pp.user_id = u.id
		WHERE u.id = ? AND u.deleted_at IS NULL
		LIMIT 1
	`
	row := r.db.QueryRow(query, userID)
	return scanUser(row)
}

// GetRegistrationToken is a no-op: registration_tokens table was removed from the schema.
func (r *authRepository) GetRegistrationToken(token string) (*domain.TokenRecord, error) {
	return nil, sql.ErrNoRows
}

// IncrementRegistrationTokenUsage is a no-op: registration_tokens table was removed from the schema.
func (r *authRepository) IncrementRegistrationTokenUsage(token string) error {
	return nil
}

func (r *authRepository) StoreRefreshToken(userID int, tokenHash string, expiresAt time.Time) error {
	var nextID int
	err := r.db.QueryRow(`SELECT COALESCE(MAX(id), 0) + 1 FROM jwt_tokens`).Scan(&nextID)
	if err != nil {
		return fmt.Errorf("failed to get next id: %w", err)
	}
	_, err = r.db.Exec(
		`INSERT INTO jwt_tokens (id, user_id, token_hash, expires_at) VALUES (?, ?, ?, ?)`,
		nextID, userID, tokenHash, expiresAt,
	)
	return err
}

func (r *authRepository) GetRefreshToken(tokenHash string) (*domain.TokenRecord, error) {
	row := r.db.QueryRow(
		`SELECT id, user_id, is_revoked, expires_at FROM jwt_tokens WHERE token_hash = ? AND is_revoked = 0 LIMIT 1`,
		tokenHash,
	)
	var rec domain.TokenRecord
	var expiresAt sql.NullTime
	if err := row.Scan(&rec.TokenID, &rec.UserID, &rec.IsRevoked, &expiresAt); err != nil {
		return nil, err
	}
	if expiresAt.Valid {
		rec.ExpiresAt = &expiresAt.Time
	}
	rec.TokenHash = tokenHash
	rec.TokenType = "refresh"
	rec.UsageLimit = nil
	rec.UsedCount = 0
	// Track last usage as required by jwt_tokens.last_used_at schema contract.
	_, _ = r.db.Exec(`UPDATE jwt_tokens SET last_used_at = NOW() WHERE token_hash = ?`, tokenHash)
	return &rec, nil
}

func (r *authRepository) RevokeRefreshToken(tokenHash string) error {
	_, err := r.db.Exec(`UPDATE jwt_tokens SET is_revoked = true WHERE token_hash = ?`, tokenHash)
	return err
}

func (r *authRepository) RevokeRefreshTokensByUserID(userID int) error {
	_, err := r.db.Exec(`UPDATE jwt_tokens SET is_revoked = true WHERE user_id = ?`, userID)
	return err
}

// LogLoginAttempt logs successful and failed login attempts to the activity_logs table.
func (r *authRepository) LogLoginAttempt(attempt domain.LoginAttempt) error {
	var userID *int
	if attempt.UserID != nil {
		userID = attempt.UserID
	}

	activityType := "login"
	description := "Login berhasil"
	if !attempt.Success {
		activityType = "login_failed"
		if attempt.EmailAttempted != nil {
			description = "Login gagal untuk email/identifier: " + *attempt.EmailAttempted
		} else {
			description = "Login gagal: password salah"
		}
	}

	_, err := r.db.Exec(`
		INSERT INTO activity_logs (user_id, activity_type, description, ip_address, user_agent)
		VALUES (?, ?, ?, ?, ?)
	`, userID, activityType, description, attempt.IPAddress, attempt.UserAgent)
	return err
}

// GetTeacherSubRoles fetches active sub-roles for a teacher user.
// Maps DB role_name to frontend SubRole format.
func (r *authRepository) GetTeacherSubRoles(userID int) []string {
	rows, err := r.db.Query(`
		SELECT tr.role_name
		FROM teacher_roles tr
		JOIN teacher_profiles tp ON tp.id = tr.teacher_id
		WHERE tp.user_id = ? AND tr.status = 'active'
	`, userID)
	if err != nil {
		return []string{}
	}
	defer rows.Close()

	roleMap := map[string]string{
		"guru_mapel": "Mapel",
		"wali_kelas": "Walkes",
		"tatib":      "Tatib",
		"kapro":      "Kapro",
	}

	var subRoles []string
	for rows.Next() {
		var roleName string
		if err := rows.Scan(&roleName); err == nil {
			if mapped, ok := roleMap[roleName]; ok {
				subRoles = append(subRoles, mapped)
			}
		}
	}
	if subRoles == nil {
		return []string{}
	}
	return subRoles
}

func scanUser(row *sql.Row) (*domain.User, error) {
	var user domain.User
	if err := row.Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.Role,
		&user.Status,
		&user.PasswordHash,
		&user.FullName,
		&user.StudentCode,
		&user.EmployeeCode,
		&user.Gender,
		&user.PhoneNumber,
		&user.ClassID,
		&user.CanRequestDispensasi,
		&user.ProfileCompleted,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.SignatureURL,
	); err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *authRepository) CreatePasswordResetToken(userID int, otpCode string, expiresAt time.Time, ip string) error {
	// Invalidate any previous unused OTPs for this user.
	// Schema uses used_at IS NULL to denote an active (unused) token.
	_, _ = r.db.Exec(`UPDATE password_reset_tokens SET used_at = NOW() WHERE user_id = ? AND used_at IS NULL`, userID)
	_, err := r.db.Exec(
		`INSERT INTO password_reset_tokens (user_id, otp_code, expires_at, ip_address) VALUES (?, ?, ?, ?)`,
		userID, otpCode, expiresAt, ip,
	)
	return err
}

func (r *authRepository) VerifyPasswordResetOTP(email, otpCode string) (int, error) {
	var userID int
	err := r.db.QueryRow(`
		SELECT prt.user_id FROM password_reset_tokens prt
		JOIN users u ON u.id = prt.user_id
		WHERE u.email = ? AND prt.otp_code = ? AND prt.used_at IS NULL AND prt.expires_at > NOW()
		LIMIT 1
	`, email, otpCode).Scan(&userID)
	if err != nil {
		return 0, err
	}
	return userID, nil
}

func (r *authRepository) MarkPasswordResetUsed(email, otpCode string) error {
	_, err := r.db.Exec(`
		UPDATE password_reset_tokens prt
		JOIN users u ON u.id = prt.user_id
		SET prt.used_at = NOW()
		WHERE u.email = ? AND prt.otp_code = ? AND prt.used_at IS NULL
	`, email, otpCode)
	return err
}

func (r *authRepository) UpdatePassword(userID int, passwordHash string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, userID); err != nil {
		return err
	}

	if _, err := tx.Exec(
		`UPDATE jwt_tokens SET is_revoked = 1, revoked_at = NOW(), revoked_reason = 'password_changed'
		 WHERE user_id = ? AND is_revoked = 0`,
		userID,
	); err != nil {
		return err
	}

	return tx.Commit()
}
