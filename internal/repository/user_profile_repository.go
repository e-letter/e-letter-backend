package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Refliqx/backend-eletter/internal/domain"
)

type UserProfileRepository interface {
	domain.UserProfileRepository
}

type userProfileRepository struct {
	db *sql.DB
}

func NewUserProfileRepository(db *sql.DB) UserProfileRepository {
	return &userProfileRepository{db: db}
}

func (r *userProfileRepository) GetByUserID(userID int) (*domain.User, error) {
	if userID == 0 {
		return &domain.User{
			ID:                   0,
			Username:             strPtr("admin"),
			Email:                strPtr("admin@system"),
			Role:                 "admin",
			Status:               "active",
			FullName:             strPtr("Administrator"),
			CanRequestDispensasi: true,
			ProfileCompleted:     true,
		}, nil
	}

	query := `
    SELECT u.id, u.username, u.email, u.role, u.status, u.password_hash,
           CASE WHEN u.role = 'teacher' THEN tp.full_name
                WHEN u.role = 'kepala_sekolah' THEN pp.full_name
                WHEN u.role = 'student' THEN sp.full_name
                WHEN u.role = 'admin' THEN u.username
           END as full_name,
           CASE WHEN u.role = 'student' THEN sp.student_code ELSE NULL END as student_code,
           CASE WHEN u.role = 'teacher' THEN tp.employee_code
                WHEN u.role = 'kepala_sekolah' THEN pp.employee_code
                ELSE NULL
           END as employee_code,
           COALESCE(tp.gender, sp.gender, pp.gender, NULL) as gender,
           COALESCE(tp.phone, sp.phone, pp.phone, NULL) as phone_number,
		       CASE WHEN u.role = 'student'
		            THEN (SELECT sce.class_id
		                  FROM student_class_enrollments sce
		                  JOIN student_profiles sp2 ON sp2.id = sce.student_id
		                  WHERE sp2.user_id = u.id AND sce.is_active = 1
		                  LIMIT 1)
		            ELSE NULL
		       END as class_id,
           CASE WHEN u.role IN ('teacher','kepala_sekolah') THEN true ELSE false END as can_request_dispensasi,
           CASE WHEN u.role = 'admin' THEN true
                WHEN u.role = 'teacher' THEN
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
           COALESCE(tp.signature_url, sp.signature_url, pp.signature_url, NULL) as signature_url,
           u.updated_at as password_changed_at
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

func strPtr(s string) *string {
	return &s
}

func (r *userProfileRepository) Update(userID int, payload domain.UserProfileUpdateRequest) (*domain.User, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if payload.Email != nil {
		if _, err := tx.Exec(`UPDATE users SET email = ?, updated_at = NOW() WHERE id = ?`, *payload.Email, userID); err != nil {
			return nil, err
		}
	}

	var userRole string
	if err := tx.QueryRow(`SELECT role FROM users WHERE id = ?`, userID).Scan(&userRole); err != nil {
		return nil, err
	}

	if userRole == "admin" {
		return r.GetByUserID(userID)
	}

	if payload.ClassID != nil && userRole == "student" {
		var studentProfileID int
		err := tx.QueryRow(`SELECT id FROM student_profiles WHERE user_id = ?`, userID).Scan(&studentProfileID)
		if err != nil {
			if err != sql.ErrNoRows {
				return nil, err
			}

			resolvedFullName := ""
			if payload.FullName != nil {
				resolvedFullName = strings.TrimSpace(*payload.FullName)
			}

			if resolvedFullName == "" {
				var email sql.NullString
				var username sql.NullString
				if err := tx.QueryRow(`SELECT email, username FROM users WHERE id = ? LIMIT 1`, userID).Scan(&email, &username); err != nil {
					return nil, err
				}
				if username.Valid && strings.TrimSpace(username.String) != "" {
					resolvedFullName = strings.TrimSpace(username.String)
				} else if email.Valid && strings.TrimSpace(email.String) != "" {
					resolvedFullName = strings.TrimSpace(strings.Split(email.String, "@")[0])
				}
			}

			if resolvedFullName == "" {
				resolvedFullName = "Siswa"
			}

			res, err2 := tx.Exec(
				`INSERT INTO student_profiles (user_id, full_name, active) VALUES (?, ?, 0)
				 ON DUPLICATE KEY UPDATE full_name = COALESCE(NULLIF(full_name, ''), VALUES(full_name))`,
				userID, resolvedFullName,
			)
			if err2 != nil {
				return nil, err2
			}

			id, _ := res.LastInsertId()
			studentProfileID = int(id)
			if studentProfileID == 0 {
				if err := tx.QueryRow(`SELECT id FROM student_profiles WHERE user_id = ?`, userID).Scan(&studentProfileID); err != nil {
					return nil, err
				}
			}
		}

		var academicYearID int
		err = tx.QueryRow(`SELECT id FROM academic_years WHERE is_active = 1 LIMIT 1`).Scan(&academicYearID)
		if err != nil {
			_ = tx.QueryRow(`SELECT id FROM academic_years ORDER BY id DESC LIMIT 1`).Scan(&academicYearID)
		}
		if academicYearID > 0 {
			_, err = tx.Exec(
				`INSERT INTO student_class_enrollments (student_id, class_id, academic_year_id, is_active)
				 VALUES (?, ?, ?, 1)
				 ON DUPLICATE KEY UPDATE class_id = VALUES(class_id), is_active = 1`,
				studentProfileID, *payload.ClassID, academicYearID,
			)
			if err != nil {
				return nil, err
			}
		}
	}

	if payload.SchoolName != nil {
		_, err = tx.Exec(
			`INSERT INTO school_config (config_key, config_value) VALUES ('school_name', ?)
			 ON DUPLICATE KEY UPDATE config_value = VALUES(config_value)`,
			*payload.SchoolName,
		)
		if err != nil {
			return nil, err
		}
	}

	updates := []string{}
	args := []any{}

	if payload.FullName != nil {
		updates = append(updates, "full_name = ?")
		args = append(args, *payload.FullName)
	}
	if payload.PhoneNumber != nil {
		updates = append(updates, "phone = ?")
		args = append(args, *payload.PhoneNumber)
	}
	if payload.Gender != nil {
		updates = append(updates, "gender = ?")
		args = append(args, *payload.Gender)
	}
	if payload.NISN != nil && userRole == "student" {
		updates = append(updates, "student_code = ?")
		args = append(args, *payload.NISN)
	}
	if payload.NIP != nil && (userRole == "teacher" || userRole == "kepala_sekolah") {
		updates = append(updates, "employee_code = ?")
		args = append(args, *payload.NIP)
	}
	if payload.SignatureUrl != nil {
		updates = append(updates, "signature_url = ?")
		args = append(args, *payload.SignatureUrl)
	}

	if payload.MarkProfileFinish {
		updates = append(updates, "active = true")
	}

	if len(updates) > 0 {
		var tableName string
		if userRole == "student" {
			tableName = "student_profiles"
		} else if userRole == "kepala_sekolah" {
			tableName = "principal_profiles"
		} else {
			tableName = "teacher_profiles"
		}

		var existingProfileID int
		profileErr := tx.QueryRow(fmt.Sprintf(`SELECT id FROM %s WHERE user_id = ? LIMIT 1`, tableName), userID).Scan(&existingProfileID)
		if profileErr == nil {
			updates = append(updates, "updated_at = NOW()")
			query := fmt.Sprintf(`UPDATE %s SET %s WHERE user_id = ?`, tableName, strings.Join(updates, ", "))
			finalArgs := append(args, userID)
			if _, err := tx.Exec(query, finalArgs...); err != nil {
				return nil, err
			}
		} else {
			if profileErr != sql.ErrNoRows {
				return nil, profileErr
			}

			if payload.FullName == nil || strings.TrimSpace(*payload.FullName) == "" {
				return nil, fmt.Errorf("nama lengkap diperlukan untuk membuat profil")
			}

			columns := []string{"user_id", "full_name"}
			values := []any{userID, *payload.FullName}

			if payload.PhoneNumber != nil {
				columns = append(columns, "phone")
				values = append(values, *payload.PhoneNumber)
			}
			if payload.Gender != nil {
				columns = append(columns, "gender")
				values = append(values, *payload.Gender)
			}
			if payload.NISN != nil && userRole == "student" {
				columns = append(columns, "student_code")
				values = append(values, *payload.NISN)
			}
			if payload.NIP != nil && (userRole == "teacher" || userRole == "kepala_sekolah") {
				columns = append(columns, "employee_code")
				values = append(values, *payload.NIP)
			}
			if payload.SignatureUrl != nil && (userRole == "teacher" || userRole == "kepala_sekolah") {
				columns = append(columns, "signature_url")
				values = append(values, *payload.SignatureUrl)
			}

			columns = append(columns, "active")
			values = append(values, payload.MarkProfileFinish)

			placeholders := make([]string, len(columns))
			for i := range columns {
				placeholders[i] = "?"
			}

			query := fmt.Sprintf(
				`INSERT INTO %s (%s) VALUES (%s)`,
				tableName,
				strings.Join(columns, ", "),
				strings.Join(placeholders, ", "),
			)

			if _, err := tx.Exec(query, values...); err != nil {
				return nil, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetByUserID(userID)
}

func (r *userProfileRepository) CompleteTeacherOnboarding(payload domain.CompleteTeacherOnboardingPayload) (*domain.User, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var teacherID int64
	err = tx.QueryRow(`SELECT id FROM teacher_profiles WHERE user_id = ?`, payload.UserID).Scan(&teacherID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			res, err := tx.Exec(
				`INSERT INTO teacher_profiles (user_id, full_name, employee_code, gender, phone, signature_url, active)
				 VALUES (?, ?, ?, ?, ?, ?, 1)`,
				payload.UserID, payload.FullName, payload.NIP, payload.Gender, payload.Phone, payload.SignatureUrl,
			)
			if err != nil {
				return nil, fmt.Errorf("gagal membuat profil guru: %w", err)
			}
			teacherID, err = res.LastInsertId()
			if err != nil {
				return nil, fmt.Errorf("gagal mendapatkan ID profil guru baru: %w", err)
			}
		} else {
			return nil, fmt.Errorf("gagal memeriksa profil guru: %w", err)
		}
	} else {
		_, err = tx.Exec(
			`UPDATE teacher_profiles
			 SET full_name = ?, employee_code = ?, gender = ?, phone = ?, signature_url = ?, active = 1, updated_at = NOW()
			 WHERE id = ?`,
			payload.FullName, payload.NIP, payload.Gender, payload.Phone, payload.SignatureUrl, teacherID,
		)
		if err != nil {
			return nil, fmt.Errorf("gagal mengupdate profil guru: %w", err)
		}
	}

	var academicYearID int64
	err = tx.QueryRow(`SELECT id FROM academic_years WHERE is_active = 1 LIMIT 1`).Scan(&academicYearID)
	if err != nil {
		_ = tx.QueryRow(`SELECT id FROM academic_years ORDER BY id DESC LIMIT 1`).Scan(&academicYearID)
	}
	if academicYearID == 0 {
		return nil, fmt.Errorf("tidak ada tahun ajaran aktif")
	}

	_, _ = tx.Exec(`DELETE FROM teacher_roles WHERE teacher_id = ?`, teacherID)
	_, _ = tx.Exec(`UPDATE class_homeroom_assignments SET is_active = 0 WHERE teacher_id = ? AND academic_year_id = ?`, teacherID, academicYearID)
	_, _ = tx.Exec(`UPDATE major_head_assignments SET is_active = 0 WHERE teacher_id = ? AND academic_year_id = ?`, teacherID, academicYearID)
	_, _ = tx.Exec(`DELETE FROM teacher_subjects WHERE teacher_id = ? AND academic_year_id = ?`, teacherID, academicYearID)
	_, _ = tx.Exec(`DELETE FROM schedules WHERE teacher_id = ? AND academic_year_id = ?`, teacherID, academicYearID)

	expectedRoles := make(map[string]struct{}, len(payload.Roles))
	for _, roleName := range payload.Roles {
		expectedRoles[roleName] = struct{}{}
	}

	var expectedSubjectIDsStr sql.NullString
	if _, ok := expectedRoles["guru_mapel"]; ok {
		if len(payload.Subjects) == 0 {
			return nil, fmt.Errorf("guru_mapel dipilih tetapi tidak ada subjects")
		}
		parts := make([]string, len(payload.Subjects))
		for i, id := range payload.Subjects {
			parts[i] = strconv.Itoa(id)
		}
		expectedSubjectIDsStr.String = strings.Join(parts, ",")
		expectedSubjectIDsStr.Valid = true
	}

	for _, roleName := range payload.Roles {
		var homeroomClassID sql.NullInt64
		var majorID sql.NullInt64
		var subjectIDsStr sql.NullString

		switch roleName {
		case "wali_kelas":
			if payload.HomeroomClassID <= 0 {
				return nil, fmt.Errorf("wali_kelas dipilih tetapi homeroom_class_id tidak valid")
			}
			homeroomClassID.Int64 = int64(payload.HomeroomClassID)
			homeroomClassID.Valid = true

			_, err = tx.Exec(
				`UPDATE class_homeroom_assignments SET is_active = 0, updated_at = NOW()
				 WHERE class_id = ? AND academic_year_id = ?`,
				payload.HomeroomClassID, academicYearID,
			)
			if err != nil {
				return nil, fmt.Errorf("gagal menonaktifkan wali kelas lama: %w", err)
			}

			_, err = tx.Exec(
				`INSERT INTO class_homeroom_assignments (class_id, teacher_id, academic_year_id, is_active)
				 VALUES (?, ?, ?, 1)
				 ON DUPLICATE KEY UPDATE teacher_id = VALUES(teacher_id), is_active = 1, updated_at = NOW()`,
				payload.HomeroomClassID, teacherID, academicYearID,
			)
			if err != nil {
				return nil, fmt.Errorf("gagal menyimpan wali kelas assignment: %w", err)
			}

		case "kapro":
			if payload.MajorID <= 0 {
				return nil, fmt.Errorf("kapro dipilih tetapi major_id tidak valid")
			}
			majorID.Int64 = int64(payload.MajorID)
			majorID.Valid = true

			_, err = tx.Exec(
				`UPDATE major_head_assignments SET is_active = 0, updated_at = NOW()
				 WHERE major_id = ? AND academic_year_id = ?`,
				payload.MajorID, academicYearID,
			)
			if err != nil {
				return nil, fmt.Errorf("gagal menonaktifkan kapro lama: %w", err)
			}

			_, err = tx.Exec(
				`INSERT INTO major_head_assignments (major_id, teacher_id, academic_year_id, is_active)
				 VALUES (?, ?, ?, 1)
				 ON DUPLICATE KEY UPDATE teacher_id = VALUES(teacher_id), is_active = 1, updated_at = NOW()`,
				payload.MajorID, teacherID, academicYearID,
			)
			if err != nil {
				return nil, fmt.Errorf("gagal menyimpan kepala program assignment: %w", err)
			}

		case "guru_mapel":
			if !expectedSubjectIDsStr.Valid {
				return nil, fmt.Errorf("guru_mapel dipilih tetapi subject staging tidak valid")
			}
			subjectIDsStr = expectedSubjectIDsStr

			for _, subjID := range payload.Subjects {
				_, err = tx.Exec(
					`INSERT INTO teacher_subjects (teacher_id, subject_id, academic_year_id, is_permanent, is_active)
					 VALUES (?, ?, ?, 0, 1)
					 ON DUPLICATE KEY UPDATE is_active = 1, updated_at = NOW()`,
					teacherID, subjID, academicYearID,
				)
				if err != nil {
					return nil, fmt.Errorf("gagal menyimpan subject guru: %w", err)
				}
			}

			for _, sched := range payload.Schedules {
				_, err = tx.Exec(
					`INSERT INTO schedules (academic_year_id, class_id, subject_id, teacher_id, day_of_week, start_time, end_time, is_active)
					 VALUES (?, ?, ?, ?, ?, ?, ?, 1)`,
					academicYearID, sched.ClassID, sched.SubjectID, teacherID, sched.DayOfWeek, sched.StartTime, sched.EndTime,
				)
				if err != nil {
					return nil, fmt.Errorf("gagal menyimpan jadwal mengajar: %w", err)
				}
			}

		case "tatib":
			break

		case "pembina":
			break

		case "tu":
			break

		default:
			return nil, fmt.Errorf("role guru tidak dikenal: %s", roleName)
		}

		_, err = tx.Exec(
			`INSERT INTO teacher_roles (teacher_id, role_name, academic_year_id, status, homeroom_class_id, major_id, subject_ids)
			 VALUES (?, ?, ?, 'active', ?, ?, ?)
			 ON DUPLICATE KEY UPDATE status = 'active', homeroom_class_id = VALUES(homeroom_class_id), major_id = VALUES(major_id), subject_ids = VALUES(subject_ids), updated_at = NOW()`,
			teacherID, roleName, academicYearID, homeroomClassID, majorID, subjectIDsStr,
		)
		if err != nil {
			return nil, fmt.Errorf("gagal menyimpan peran guru: %w", err)
		}
	}

	for roleName := range expectedRoles {
		var count int
		err = tx.QueryRow(`
			SELECT COUNT(1)
			FROM teacher_roles
			WHERE teacher_id = ? AND role_name = ? AND academic_year_id = ? AND status = 'active'
		`, teacherID, roleName, academicYearID).Scan(&count)
		if err != nil {
			return nil, fmt.Errorf("verifikasi teacher_roles gagal untuk %s: %w", roleName, err)
		}
		if count != 1 {
			return nil, fmt.Errorf("verifikasi teacher_roles gagal untuk %s: expected 1 active row, got %d", roleName, count)
		}
	}

	if _, ok := expectedRoles["wali_kelas"]; ok {
		var got sql.NullInt64
		if err := tx.QueryRow(`
			SELECT homeroom_class_id
			FROM teacher_roles
			WHERE teacher_id = ? AND role_name = 'wali_kelas' AND academic_year_id = ? AND status = 'active'
		`, teacherID, academicYearID).Scan(&got); err != nil {
			return nil, fmt.Errorf("verifikasi homeroom_class_id gagal: %w", err)
		}
		if !got.Valid || got.Int64 != int64(payload.HomeroomClassID) {
			return nil, fmt.Errorf("homeroom_class_id tidak sesuai (expected %d, got %v)", payload.HomeroomClassID, got)
		}
	}

	if _, ok := expectedRoles["kapro"]; ok {
		var got sql.NullInt64
		if err := tx.QueryRow(`
			SELECT major_id
			FROM teacher_roles
			WHERE teacher_id = ? AND role_name = 'kapro' AND academic_year_id = ? AND status = 'active'
		`, teacherID, academicYearID).Scan(&got); err != nil {
			return nil, fmt.Errorf("verifikasi major_id gagal: %w", err)
		}
		if !got.Valid || got.Int64 != int64(payload.MajorID) {
			return nil, fmt.Errorf("major_id tidak sesuai (expected %d, got %v)", payload.MajorID, got)
		}
	}

	if _, ok := expectedRoles["guru_mapel"]; ok {
		var got sql.NullString
		if err := tx.QueryRow(`
			SELECT subject_ids
			FROM teacher_roles
			WHERE teacher_id = ? AND role_name = 'guru_mapel' AND academic_year_id = ? AND status = 'active'
		`, teacherID, academicYearID).Scan(&got); err != nil {
			return nil, fmt.Errorf("verifikasi subject_ids gagal: %w", err)
		}
		if !got.Valid || strings.TrimSpace(got.String) != strings.TrimSpace(expectedSubjectIDsStr.String) {
			return nil, fmt.Errorf("subject_ids tidak sesuai (expected %s, got %v)", expectedSubjectIDsStr.String, got)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return r.GetByUserID(payload.UserID)
}

func (r *userProfileRepository) GetSchedules(userID int) ([]domain.ScheduleDetail, error) {
	var academicYearID int
	err := r.db.QueryRow(`SELECT id FROM academic_years WHERE is_active = 1 LIMIT 1`).Scan(&academicYearID)
	if err != nil {
		return nil, fmt.Errorf("gagal mendapatkan tahun ajaran aktif: %w", err)
	}

	rows, err := r.db.Query(`
		SELECT class_id, subject_id, day_of_week, start_time, end_time
		FROM schedules
		WHERE teacher_id = (SELECT id FROM teacher_profiles WHERE user_id = ? AND deleted_at IS NULL)
		  AND academic_year_id = ?
		  AND is_active = 1
	`, userID, academicYearID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []domain.ScheduleDetail
	for rows.Next() {
		var s domain.ScheduleDetail
		var startTime, endTime string
		if err := rows.Scan(&s.ClassID, &s.SubjectID, &s.DayOfWeek, &startTime, &endTime); err != nil {
			return nil, err
		}
		s.StartTime = formatTime(startTime)
		s.EndTime = formatTime(endTime)
		schedules = append(schedules, s)
	}
	return schedules, nil
}

func formatTime(t string) string {
	parts := strings.Split(t, ":")
	if len(parts) >= 2 {
		return parts[0] + ":" + parts[1]
	}
	return t
}
