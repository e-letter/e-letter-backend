package repository

import (
	"database/sql"
	"time"

	"github.com/Refliqx/backend-eletter/internal/domain"
)

func (r *authRepository) GetUserByLoginIdentifiers(id string) (*domain.User, error) {
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
		       CASE WHEN u.role = 'teacher' THEN COALESCE(tp.active, 0)
		            WHEN u.role = 'kepala_sekolah' THEN COALESCE(pp.active, 0)
		            WHEN u.role = 'student' THEN COALESCE(sp.active, 0)
		            ELSE false
		       END as profile_completed
		FROM users u
		LEFT JOIN teacher_profiles tp ON tp.user_id = u.id
		LEFT JOIN student_profiles sp ON sp.user_id = u.id
		LEFT JOIN principal_profiles pp ON pp.user_id = u.id
		WHERE (u.username = ? OR u.email = ? OR sp.student_code = ? OR tp.employee_code = ? OR pp.employee_code = ?)
		  AND u.status = 'active' AND u.deleted_at IS NULL
		LIMIT 1
	`
	row := r.db.QueryRow(query, id, id, id, id, id)
	return scanUser(row)
}

func (r *authRepository) GetUserByEmail(email string) (*domain.User, error) {
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
		       CASE WHEN u.role = 'teacher' THEN COALESCE(tp.active, 0)
		            WHEN u.role = 'kepala_sekolah' THEN COALESCE(pp.active, 0)
		            WHEN u.role = 'student' THEN COALESCE(sp.active, 0)
		            ELSE false
		       END as profile_completed
		FROM users u
		LEFT JOIN teacher_profiles tp ON tp.user_id = u.id
		LEFT JOIN student_profiles sp ON sp.user_id = u.id
		LEFT JOIN principal_profiles pp ON pp.user_id = u.id
		WHERE u.email = ? AND u.status = 'active' AND u.deleted_at IS NULL
		LIMIT 1
	`
	var user domain.User
	if err := r.db.QueryRow(query, email).Scan(
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
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *authRepository) CreateUser(roleID int, email, passwordHash string) (int, error) {
	role := "student"
	if roleID == 2 {
		role = "teacher"
	}
	res, err := r.db.Exec(
		`INSERT INTO users (email, password_hash, role, status) VALUES (?, ?, ?, 'active')`,
		email, passwordHash, role,
	)
	if err != nil {
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
		       CASE WHEN u.role = 'teacher' THEN COALESCE(tp.active, 0)
		            WHEN u.role = 'kepala_sekolah' THEN COALESCE(pp.active, 0)
		            WHEN u.role = 'student' THEN COALESCE(sp.active, 0)
		            ELSE false
		       END as profile_completed
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

func (r *authRepository) GetRegistrationToken(token string) (*domain.TokenRecord, error) {
	row := r.db.QueryRow(
		`SELECT token_id, 0 AS user_id, token, expires_at, used_count, usage_limit
		 FROM registration_tokens WHERE token = ? AND used_count < usage_limit AND (expires_at IS NULL OR expires_at > NOW()) LIMIT 1`,
		token,
	)
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

func (r *authRepository) IncrementRegistrationTokenUsage(token string) error {
	_, err := r.db.Exec(`UPDATE registration_tokens SET used_count = used_count + 1 WHERE token = ?`, token)
	return err
}

func (r *authRepository) StoreRefreshToken(userID int, tokenHash string, expiresAt time.Time) error {
	_, err := r.db.Exec(
		`INSERT INTO jwt_tokens (user_id, token_hash, expires_at) VALUES (?, ?, ?)`,
		userID, tokenHash, expiresAt,
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

// LogLoginAttempt is a no-op: login_logs table does not exist in the schema.
func (r *authRepository) LogLoginAttempt(attempt domain.LoginAttempt) error {
	return nil
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
	); err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *authRepository) CreatePasswordResetToken(userID int, otpHash string, expiresAt time.Time, ip string) error {
	// Invalidate previous tokens for this user
	_, _ = r.db.Exec(`UPDATE password_reset_tokens SET is_used = 1 WHERE user_id = ? AND is_used = 0`, userID)
	_, err := r.db.Exec(
		`INSERT INTO password_reset_tokens (user_id, token_hash, expires_at, ip_address) VALUES (?, ?, ?, ?)`,
		userID, otpHash, expiresAt, ip,
	)
	return err
}

func (r *authRepository) VerifyPasswordResetOTP(email, otpHash string) (int, error) {
	var userID int
	err := r.db.QueryRow(`
		SELECT prt.user_id FROM password_reset_tokens prt
		JOIN users u ON u.id = prt.user_id
		WHERE u.email = ? AND prt.token_hash = ? AND prt.is_used = 0 AND prt.expires_at > NOW()
		LIMIT 1
	`, email, otpHash).Scan(&userID)
	if err != nil {
		return 0, err
	}
	return userID, nil
}

func (r *authRepository) MarkPasswordResetUsed(email, otpHash string) error {
	_, err := r.db.Exec(`
		UPDATE password_reset_tokens prt
		JOIN users u ON u.id = prt.user_id
		SET prt.is_used = 1, prt.used_at = NOW()
		WHERE u.email = ? AND prt.token_hash = ?
	`, email, otpHash)
	return err
}

func (r *authRepository) UpdatePassword(userID int, passwordHash string) error {
	_, err := r.db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, userID)
	return err
}
