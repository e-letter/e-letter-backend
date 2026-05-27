package repository

import (
	"database/sql"
	"fmt"
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
	// Special case for admin with userID=0 (no database user record)
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
           CASE WHEN u.role IN ('teacher','kepala_sekolah') THEN tp.full_name
                WHEN u.role = 'student' THEN sp.full_name
                WHEN u.role = 'admin' THEN u.username
           END as full_name,
           CASE WHEN u.role = 'student' THEN sp.student_code ELSE NULL END as student_code,
           CASE WHEN u.role IN ('teacher','kepala_sekolah') THEN tp.employee_code ELSE NULL END as employee_code,
           COALESCE(tp.gender, sp.gender, NULL) as gender,
           COALESCE(tp.phone, sp.phone, NULL) as phone_number,
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
                WHEN u.role IN ('teacher','kepala_sekolah') THEN COALESCE(tp.active, 0)
                WHEN u.role = 'student' THEN COALESCE(sp.active, 0)
                ELSE false
           END as profile_completed
    FROM users u
    LEFT JOIN teacher_profiles tp ON tp.user_id = u.id
    LEFT JOIN student_profiles sp ON sp.user_id = u.id
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
		if _, err := tx.Exec(`UPDATE users SET email = ? WHERE id = ?`, *payload.Email, userID); err != nil {
			return nil, err
		}
	}

	// Determine which table to update based on user role
	var userRole string
	if err := tx.QueryRow(`SELECT role FROM users WHERE id = ?`, userID).Scan(&userRole); err != nil {
		return nil, err
	}

	// Admin users don't have profiles - just return current user
	if userRole == "admin" {
		return r.GetByUserID(userID)
	}

	// Handle class_id via student_class_enrollments (not direct column)
	if payload.ClassID != nil && userRole == "student" {
		// Get student profile ID
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

		// Get active academic year
		var academicYearID int
		err = tx.QueryRow(`SELECT id FROM academic_years WHERE is_active = 1 LIMIT 1`).Scan(&academicYearID)
		if err != nil {
			// Fallback to latest
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
	if payload.NISN != nil {
		updates = append(updates, "student_code = ?")
		args = append(args, *payload.NISN)
	}
	if payload.NIP != nil {
		updates = append(updates, "employee_code = ?")
		args = append(args, *payload.NIP)
	}
	if payload.SchoolName != nil {
		updates = append(updates, "school_name = ?")
		args = append(args, *payload.SchoolName)
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
		} else {
			tableName = "teacher_profiles"
		}

		var existingProfileID int
		profileErr := tx.QueryRow(fmt.Sprintf(`SELECT id FROM %s WHERE user_id = ? LIMIT 1`, tableName), userID).Scan(&existingProfileID)
		if profileErr == nil {
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
				return nil, fmt.Errorf("nama lengkap diperlukan untuk membuat profil siswa")
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
			if payload.NISN != nil {
				columns = append(columns, "student_code")
				values = append(values, *payload.NISN)
			}
			if payload.NIP != nil {
				columns = append(columns, "employee_code")
				values = append(values, *payload.NIP)
			}
			if payload.SchoolName != nil {
				columns = append(columns, "school_name")
				values = append(values, *payload.SchoolName)
			}
			if payload.SignatureUrl != nil {
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
