package service

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Refliqx/backend-eletter/internal/domain"
	"github.com/Refliqx/backend-eletter/internal/utils"
	"github.com/go-sql-driver/mysql"
)

type AdminService interface {
	domain.AdminService
}

type adminService struct {
	db *sql.DB
}

func NewAdminService(db *sql.DB) AdminService {
	return &adminService{db: db}
}

var knownRoles = map[string]bool{
	"student":        true,
	"teacher":        true,
	"kepala_sekolah": true,
	"admin":          true,
}

func roleProfileTable(role string) string {
	switch role {
	case "student":
		return "student_profiles"
	case "teacher":
		return "teacher_profiles"
	case "kepala_sekolah":
		return "principal_profiles"
	case "admin":
		return "admin_profiles"
	case "tu":
		return "tu_profiles"
	}
	return ""
}

func roleProfileHasDeletedAt(role string) bool {
	return role == "student" || role == "teacher"
}

func (s *adminService) GetStats() (map[string]int, error) {
	stats := map[string]int{}
	rows := []struct{ key, query string }{
		{"total_students", `SELECT COUNT(*) FROM users WHERE role='student' AND deleted_at IS NULL`},
		{"total_teachers", `SELECT COUNT(*) FROM users WHERE role IN ('teacher','kepala_sekolah') AND deleted_at IS NULL`},
		{"pending_requests", `SELECT COUNT(*) FROM requests WHERE status='pending'`},
		{"active_tokens", `SELECT COUNT(*) FROM registration_tokens WHERE used_count < usage_limit AND (expires_at IS NULL OR expires_at > NOW())`},
	}
	for _, r := range rows {
		var count int
		_ = s.db.QueryRow(r.query).Scan(&count)
		stats[r.key] = count
	}
	return stats, nil
}

func (s *adminService) GetUsers(role, status, search string, page, pageSize int) ([]domain.AdminUserListItem, int, int, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize

	baseQuery := `FROM users u
		LEFT JOIN teacher_profiles tp ON tp.user_id = u.id
		LEFT JOIN student_profiles sp ON sp.user_id = u.id
		LEFT JOIN admin_profiles ap ON ap.user_id = u.id
		LEFT JOIN principal_profiles pp ON pp.user_id = u.id
		LEFT JOIN tu_profiles tup ON tup.user_id = u.id
		WHERE u.deleted_at IS NULL AND u.role != 'admin'`
	args := []any{}

	if role != "" {
		baseQuery += " AND u.role = ?"
		args = append(args, role)
	}
	if status != "" {
		baseQuery += " AND u.status = ?"
		args = append(args, status)
	}
	if search != "" {
		baseQuery += " AND (u.email LIKE ? OR tp.full_name LIKE ? OR sp.full_name LIKE ? OR ap.full_name LIKE ? OR pp.full_name LIKE ? OR tup.full_name LIKE ?)"
		s := "%" + search + "%"
		args = append(args, s, s, s, s, s, s)
	}

	var total, activeTotal, pendingTotal int
	countQuery := "SELECT COUNT(*) " + baseQuery
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, 0, 0, err
	}

	if err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE deleted_at IS NULL AND role != 'admin' AND status = 'active'").Scan(&activeTotal); err != nil {
		return nil, 0, 0, 0, err
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE deleted_at IS NULL AND role != 'admin' AND status = 'pending'").Scan(&pendingTotal); err != nil {
		return nil, 0, 0, 0, err
	}

	selectQuery := "SELECT u.id, u.email, u.role, u.status, COALESCE(tp.full_name, sp.full_name, ap.full_name, pp.full_name, tup.full_name, '') as full_name " + baseQuery + " ORDER BY u.id DESC LIMIT ? OFFSET ?"
	argsLimit := append(args, pageSize, offset)

	rows, err := s.db.Query(selectQuery, argsLimit...)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	defer rows.Close()

	var users []domain.AdminUserListItem
	for rows.Next() {
		var u domain.AdminUserListItem
		if err := rows.Scan(&u.ID, &u.Email, &u.Role, &u.Status, &u.FullName); err != nil {
			return nil, 0, 0, 0, err
		}
		users = append(users, u)
	}
	return users, total, activeTotal, pendingTotal, nil
}

func (s *adminService) UpdateUserStatus(userID int64, status string, adminUserID int64, ip, userAgent string) error {
	var statusExists int
	if err := s.db.QueryRow(`SELECT 1 FROM ref_values WHERE group_key = 'user_status' AND value = ? AND is_active = 1`, status).Scan(&statusExists); err == sql.ErrNoRows {
		return fmt.Errorf("Status pengguna tidak valid")
	}
	_, err := s.db.Exec(`UPDATE users SET status = ? WHERE id = ?`, status, userID)
	if err != nil {
		return err
	}
	utils.LogActivity(
		s.db,
		adminUserID,
		"update_user_status",
		fmt.Sprintf("Admin mengubah status user id=%d menjadi '%s'", userID, status),
		ip,
		userAgent,
	)
	return nil
}

func (s *adminService) UpdateUser(id int64, role, fullName *string, adminUserID int64, ip, userAgent string) (map[string]any, error) {
	if role == nil && fullName == nil {
		return nil, fmt.Errorf("Tidak ada perubahan yang dikirim")
	}
	if role != nil && !knownRoles[*role] {
		return nil, fmt.Errorf("Role tidak dikenal: %s", *role)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var currentRole string
	var currentStatus string
	err = tx.QueryRow(`SELECT role, status FROM users WHERE id = ? AND deleted_at IS NULL FOR UPDATE`, id).Scan(&currentRole, &currentStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("Pengguna tidak ditemukan")
		}
		return nil, err
	}

	targetRole := currentRole
	if role != nil {
		targetRole = *role
	}

	if role != nil {
		if _, err := tx.Exec(`UPDATE users SET role = ?, updated_at = NOW() WHERE id = ?`, *role, id); err != nil {
			return nil, err
		}
	}

	newFullName := ""
	if fullName != nil {
		newFullName = strings.TrimSpace(*fullName)
	}
	if newFullName == "" {
		_ = tx.QueryRow(
			fmt.Sprintf(`SELECT full_name FROM %s WHERE user_id = ? ORDER BY id DESC LIMIT 1`, roleProfileTable(currentRole)),
			id,
		).Scan(&newFullName)
	}

	oldTable := roleProfileTable(currentRole)
	if oldTable != "" && currentRole != targetRole {
		if roleProfileHasDeletedAt(currentRole) {
			if _, err := tx.Exec(
				fmt.Sprintf(`UPDATE %s SET deleted_at = NOW(), active = 0, updated_at = NOW() WHERE user_id = ? AND deleted_at IS NULL`, oldTable),
				id,
			); err != nil {
				return nil, fmt.Errorf("Gagal menonaktifkan profil lama: %s", err.Error())
			}
		}
	}

	newTable := roleProfileTable(targetRole)
	if newTable != "" {
		if roleProfileHasDeletedAt(targetRole) {
			res, err := tx.Exec(
				fmt.Sprintf(`UPDATE %s SET full_name = ?, updated_at = NOW() WHERE user_id = ? AND deleted_at IS NULL`, newTable),
				newFullName, id,
			)
			if err != nil {
				return nil, err
			}
			affected, _ := res.RowsAffected()
			if affected == 0 {
				switch targetRole {
				case "student":
					if _, err := tx.Exec(
						`INSERT INTO student_profiles (user_id, full_name, gender, active) VALUES (?, ?, 'other', 1)`,
						id, newFullName,
					); err != nil {
						return nil, fmt.Errorf("Gagal membuat profil siswa: %s", err.Error())
					}
				case "teacher":
					if _, err := tx.Exec(
						`INSERT INTO teacher_profiles (user_id, full_name, active) VALUES (?, ?, 1)`,
						id, newFullName,
					); err != nil {
						return nil, fmt.Errorf("Gagal membuat profil guru: %s", err.Error())
					}
				}
			}
		} else {
			res, err := tx.Exec(
				fmt.Sprintf(`UPDATE %s SET full_name = ?, updated_at = NOW() WHERE user_id = ?`, newTable),
				newFullName, id,
			)
			if err != nil {
				return nil, err
			}
			affected, _ := res.RowsAffected()
			if affected == 0 {
				switch targetRole {
				case "admin":
					if _, err := tx.Exec(
						`INSERT INTO admin_profiles (user_id, full_name) VALUES (?, ?)`,
						id, newFullName,
					); err != nil {
						return nil, fmt.Errorf("Gagal membuat profil admin: %s", err.Error())
					}
				case "kepala_sekolah":
					var activeCount int
					if err := tx.QueryRow(`SELECT COUNT(*) FROM principal_profiles WHERE active = 1`).Scan(&activeCount); err != nil {
						return nil, err
					}
					if activeCount > 0 {
						return nil, fmt.Errorf("Sudah ada kepala sekolah aktif. Nonaktifkan yang lama sebelum menambah yang baru.")
					}
					if _, err := tx.Exec(
						`INSERT INTO principal_profiles (user_id, full_name, active) VALUES (?, ?, 1)`,
						id, newFullName,
					); err != nil {
						return nil, fmt.Errorf("Gagal membuat profil kepala sekolah: %s", err.Error())
					}
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	descParts := []string{fmt.Sprintf("Admin memperbarui user id=%d", id)}
	if role != nil {
		descParts = append(descParts, fmt.Sprintf("role: %s -> %s", currentRole, targetRole))
	}
	if fullName != nil {
		descParts = append(descParts, fmt.Sprintf("nama: '%s'", *fullName))
	}
	utils.LogActivity(
		s.db,
		adminUserID,
		"update_user",
		strings.Join(descParts, "; "),
		ip,
		userAgent,
	)

	return map[string]any{
		"id":        id,
		"role":      targetRole,
		"full_name": newFullName,
		"status":    currentStatus,
	}, nil
}

func (s *adminService) CreateUser(fullName, email, role, status, password string, adminUserID int64, ip, userAgent string) (map[string]any, error) {
	role = strings.ToLower(strings.TrimSpace(role))
	if !knownRoles[role] {
		return nil, fmt.Errorf("Peran tidak dikenal")
	}

	usernameRoles := map[string]string{
		"admin":          "ADM",
		"kepala_sekolah": "KS",
		"tu":             "TU",
	}
	prefix, usesUsername := usernameRoles[role]

	var username *string
	var emailPtr *string
	rawEmail := strings.ToLower(strings.TrimSpace(email))

	if usesUsername {
		var lastNum int
		err := s.db.QueryRow(
			`SELECT COALESCE(MAX(CAST(SUBSTRING_INDEX(username, '-', -1) AS UNSIGNED)), 0) FROM users WHERE username LIKE ? AND deleted_at IS NULL`,
			prefix+"-%",
		).Scan(&lastNum)
		if err != nil {
			return nil, err
		}
		generated := fmt.Sprintf("%s-%03d", prefix, lastNum+1)
		username = &generated
	} else {
		emailPtr = &rawEmail
		var existingID int
		err := s.db.QueryRow(`SELECT id FROM users WHERE email = ? AND deleted_at IS NULL`, rawEmail).Scan(&existingID)
		if err == nil {
			return nil, fmt.Errorf("Email sudah terdaftar")
		}
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
	}

	statusStr := strings.TrimSpace(status)
	if statusStr == "" {
		statusStr = "active"
	}
	if statusStr != "active" && statusStr != "pending" && statusStr != "inactive" {
		return nil, fmt.Errorf("Status tidak valid")
	}

	passwordStr := password
	if passwordStr == "" {
		passwordStr = "e-letter-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	hash, err := utils.HashPassword(passwordStr)
	if err != nil {
		return nil, fmt.Errorf("Gagal memproses kata sandi")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`INSERT INTO users (username, email, password_hash, role, status) VALUES (?, ?, ?, ?, ?)`,
		username, emailPtr, hash, role, statusStr,
	)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			identifier := "Email"
			if usesUsername {
				identifier = "Username"
			}
			return nil, fmt.Errorf("%s sudah terdaftar", identifier)
		}
		return nil, err
	}
	userID, _ := res.LastInsertId()

	profileTable := roleProfileTable(role)
	if profileTable == "" {
		return nil, fmt.Errorf("Peran tidak memiliki profil")
	}

	active := 0
	if statusStr == "active" {
		active = 1
	}

	if role == "kepala_sekolah" && active == 1 {
		var activeCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM principal_profiles WHERE active = 1`).Scan(&activeCount); err != nil {
			return nil, err
		}
		if activeCount > 0 {
			return nil, fmt.Errorf("Sudah ada kepala sekolah aktif. Nonaktifkan yang lama sebelum menambah yang baru.")
		}
	}

	var profileErr error
	switch role {
	case "student":
		_, profileErr = tx.Exec(
			`INSERT INTO student_profiles (user_id, full_name, gender, active) VALUES (?, ?, 'other', ?)`,
			userID, fullName, active,
		)
	case "teacher":
		_, profileErr = tx.Exec(
			`INSERT INTO teacher_profiles (user_id, full_name, active) VALUES (?, ?, ?)`,
			userID, fullName, active,
		)
	case "kepala_sekolah":
		_, profileErr = tx.Exec(
			`INSERT INTO principal_profiles (user_id, full_name, active) VALUES (?, ?, ?)`,
			userID, fullName, active,
		)
	case "admin":
		_, profileErr = tx.Exec(
			`INSERT INTO admin_profiles (user_id, full_name) VALUES (?, ?)`,
			userID, fullName,
		)
	case "tu":
		_, profileErr = tx.Exec(
			`INSERT INTO tu_profiles (user_id, full_name) VALUES (?, ?)`,
			userID, fullName,
		)
	}
	if profileErr != nil {
		return nil, fmt.Errorf("Gagal membuat profil: %s", profileErr.Error())
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	identifier := rawEmail
	if usesUsername {
		identifier = *username
	}

	utils.LogActivity(
		s.db,
		adminUserID,
		"create_user",
		fmt.Sprintf("Admin membuat akun baru: %s (%s) dengan role=%s, status=%s", fullName, identifier, role, statusStr),
		ip,
		userAgent,
	)
	return map[string]any{
		"id":        userID,
		"username":  username,
		"email":     emailPtr,
		"role":      role,
		"full_name": fullName,
		"status":    statusStr,
		"password":  passwordStr,
	}, nil
}

func (s *adminService) AdminDeleteLetter(id int, adminUserID int64, ip, userAgent string) (map[string]any, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var requesterUserID int64
	var requestNumber sql.NullString
	var letterType sql.NullString
	var status sql.NullString
	err = tx.QueryRow(
		`SELECT requester_user_id, request_number, letter_type, status FROM requests WHERE id = ? AND deleted_at IS NULL FOR UPDATE`,
		id,
	).Scan(&requesterUserID, &requestNumber, &letterType, &status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("Surat tidak ditemukan atau sudah dihapus")
		}
		return nil, err
	}

	if _, err := tx.Exec(`UPDATE requests SET deleted_at = NOW() WHERE id = ?`, id); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	number := "-"
	if requestNumber.Valid {
		number = requestNumber.String
	}
	utils.LogActivity(
		s.db,
		adminUserID,
		"admin_delete_letter",
		fmt.Sprintf("Admin menghapus surat id=%d nomor=%s atas permintaan user_id=%d (status=%s)", id, number, requesterUserID, status.String),
		ip,
		userAgent,
	)

	return map[string]any{
		"id":                id,
		"letter_type":       letterType.String,
		"request_number":    number,
		"requester_user_id": requesterUserID,
	}, nil
}

func (s *adminService) GetRegistrationTokens() ([]domain.AdminRegistrationToken, error) {
	rows, err := s.db.Query(`SELECT token_id, token, role_id, usage_limit, used_count, expires_at, created_at FROM registration_tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []domain.AdminRegistrationToken
	for rows.Next() {
		var t domain.AdminRegistrationToken
		if err := rows.Scan(&t.ID, &t.Token, &t.RoleID, &t.UsageLimit, &t.UsedCount, &t.ExpiresAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, nil
}

func (s *adminService) CreateRegistrationToken(token string, roleID int, usageLimit int, expiresAt *string, adminUserID int64, ip, userAgent string) error {
	if usageLimit == 0 {
		usageLimit = 100
	}
	_, err := s.db.Exec(`INSERT INTO registration_tokens (token, role_id, usage_limit, expires_at) VALUES (?, ?, ?, ?)`,
		token, roleID, usageLimit, expiresAt)
	if err != nil {
		return err
	}
	utils.LogActivity(s.db, adminUserID, "create_registration_token", "Admin membuat token registrasi: "+token, ip, userAgent)
	return nil
}

func (s *adminService) DeleteRegistrationToken(id string, adminUserID int64, ip, userAgent string) error {
	_, err := s.db.Exec(`DELETE FROM registration_tokens WHERE token_id = ?`, id)
	if err != nil {
		return err
	}
	utils.LogActivity(s.db, adminUserID, "delete_registration_token", "Admin menghapus token registrasi id="+id, ip, userAgent)
	return nil
}

func (s *adminService) VerifyTeacherRole(id string, adminUserID int64, ip, userAgent string) error {
	idInt, err := strconv.Atoi(id)
	if err != nil {
		return fmt.Errorf("Invalid ID format")
	}

	if idInt < 0 {
		userID := -idInt
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()

		var status string
		var role string
		err = tx.QueryRow(`SELECT status, role FROM users WHERE id = ? FOR UPDATE`, userID).Scan(&status, &role)
		if err != nil {
			return fmt.Errorf("User tidak ditemukan")
		}
		if role != "teacher" || status != "pending" {
			return fmt.Errorf("User bukan guru pending")
		}

		if _, err := tx.Exec(`UPDATE users SET status = 'active', updated_at = NOW() WHERE id = ?`, userID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE teacher_profiles SET active = 1, updated_at = NOW() WHERE user_id = ?`, userID); err != nil {
			return err
		}

		if err := tx.Commit(); err != nil {
			return err
		}
		utils.LogActivity(
			s.db,
			adminUserID,
			"approve_teacher_registration",
			fmt.Sprintf("Admin menyetujui pendaftaran guru user_id=%d", userID),
			ip,
			userAgent,
		)
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var roleName string
	var teacherID int
	var academicYearID int
	var homeroomClassID sql.NullInt64
	var majorID sql.NullInt64
	var subjectIDsRaw sql.NullString
	err = tx.QueryRow(
		`SELECT role_name, teacher_id, COALESCE(academic_year_id, 0),
		        homeroom_class_id, major_id, subject_ids
		   FROM teacher_roles WHERE id = ? FOR UPDATE`,
		id,
	).Scan(&roleName, &teacherID, &academicYearID, &homeroomClassID, &majorID, &subjectIDsRaw)
	if err != nil {
		return fmt.Errorf("Role tidak ditemukan")
	}

	if _, err := tx.Exec(
		`UPDATE teacher_roles SET status = 'active', verified_at = NOW(), verified_by = ? WHERE id = ?`,
		adminUserID, id,
	); err != nil {
		return err
	}

	switch roleName {
	case "wali_kelas":
		if homeroomClassID.Valid {
			if _, err := tx.Exec(
				`UPDATE class_homeroom_assignments SET is_active = 0, updated_at = NOW()
				  WHERE class_id = ? AND academic_year_id = ?`,
				homeroomClassID.Int64, academicYearID,
			); err != nil {
				return err
			}
			if _, err := tx.Exec(
				`INSERT INTO class_homeroom_assignments (class_id, teacher_id, academic_year_id, is_active)
				 VALUES (?, ?, ?, 1)
				 ON DUPLICATE KEY UPDATE teacher_id = VALUES(teacher_id), is_active = 1, updated_at = NOW()`,
				homeroomClassID.Int64, teacherID, academicYearID,
			); err != nil {
				return err
			}
		}

	case "kapro":
		if majorID.Valid {
			if _, err := tx.Exec(
				`UPDATE major_head_assignments SET is_active = 0, updated_at = NOW()
				  WHERE major_id = ? AND academic_year_id = ?`,
				majorID.Int64, academicYearID,
			); err != nil {
				return err
			}
			if _, err := tx.Exec(
				`INSERT INTO major_head_assignments (major_id, teacher_id, academic_year_id, is_active)
				 VALUES (?, ?, ?, 1)
				 ON DUPLICATE KEY UPDATE teacher_id = VALUES(teacher_id), is_active = 1, updated_at = NOW()`,
				majorID.Int64, teacherID, academicYearID,
			); err != nil {
				return err
			}
		}

	case "guru_mapel":
		if subjectIDsRaw.Valid && subjectIDsRaw.String != "" {
			classRows, err := tx.Query(
				`SELECT id FROM classes WHERE academic_year_id = ? AND is_active = 1`,
				academicYearID,
			)
			if err != nil {
				return err
			}
			var classIDs []int64
			for classRows.Next() {
				var cid int64
				_ = classRows.Scan(&cid)
				classIDs = append(classIDs, cid)
			}
			classRows.Close()

			parts := strings.Split(subjectIDsRaw.String, ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				subjectID, convErr := strconv.ParseInt(part, 10, 64)
				if convErr != nil {
					continue
				}
				for _, classID := range classIDs {
					if _, err := tx.Exec(
						`INSERT IGNORE INTO schedules
							(academic_year_id, class_id, subject_id, teacher_id,
							 day_of_week, start_time, end_time, is_active)
						VALUES (?, ?, ?, ?, 'senin', '07:00:00', '08:00:00', 1)`,
						academicYearID, classID, subjectID, teacherID,
					); err != nil {
						return fmt.Errorf("Gagal membuat jadwal untuk subject %d kelas %d: %s", subjectID, classID, err.Error())
					}
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	utils.LogActivity(
		s.db,
		adminUserID,
		"verify_teacher_role",
		fmt.Sprintf("Admin memverifikasi peran guru: role_id=%s role_name=%s teacher_id=%d", id, roleName, teacherID),
		ip,
		userAgent,
	)
	return nil
}

func (s *adminService) ListPendingTeacherRoles(status string, page, limit int) ([]domain.AdminPendingRole, int, int, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	unionSQL := `
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
		UNION ALL
		SELECT
			-CAST(u.id AS SIGNED) AS id,
			tp.id AS teacher_id,
			u.id  AS teacher_user_id,
			tp.full_name,
			'registrasi' AS role_name,
			u.status AS status,
			NULL AS homeroom_class_id,
			NULL AS class_name,
			NULL AS major_id,
			NULL AS major_name,
			NULL AS subject_ids,
			DATE_FORMAT(tp.created_at, '%Y-%m-%dT%H:%i:%sZ') AS created_at
		FROM users u
		JOIN teacher_profiles tp ON tp.user_id = u.id
		WHERE u.role = 'teacher' AND u.status = ? AND u.deleted_at IS NULL
	`

	var total int
	countSQL := "SELECT COUNT(*) FROM (" + unionSQL + ") AS combined"
	if err := s.db.QueryRow(countSQL, status, status).Scan(&total); err != nil {
		return nil, 0, 0, 0, err
	}

	listSQL := "SELECT * FROM (" + unionSQL + ") AS combined ORDER BY created_at DESC LIMIT ? OFFSET ?"
	rows, err := s.db.Query(listSQL, status, status, limit, offset)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	defer rows.Close()

	var items []domain.AdminPendingRole
	for rows.Next() {
		var rec domain.AdminPendingRole
		var classID sql.NullInt64
		var className sql.NullString
		var majorID sql.NullInt64
		var majorName sql.NullString
		var subjectIDs sql.NullString
		if err := rows.Scan(
			&rec.ID, &rec.TeacherID, &rec.TeacherUserID, &rec.TeacherName,
			&rec.RoleName, &rec.Status,
			&classID, &className,
			&majorID, &majorName,
			&subjectIDs,
			&rec.CreatedAt,
		); err != nil {
			return nil, 0, 0, 0, err
		}
		if classID.Valid {
			rec.ClassID = &classID.Int64
		}
		if className.Valid {
			rec.ClassName = &className.String
		}
		if majorID.Valid {
			rec.MajorID = &majorID.Int64
		}
		if majorName.Valid {
			rec.MajorName = &majorName.String
		}
		if subjectIDs.Valid {
			rec.SubjectIDs = &subjectIDs.String
		}
		items = append(items, rec)
	}
	if items == nil {
		items = []domain.AdminPendingRole{}
	}
	totalPages := (total + limit - 1) / limit
	return items, total, totalPages, page, nil
}

func (s *adminService) RejectTeacherRole(id string, adminUserID int64, ip, userAgent string) error {
	idInt, err := strconv.Atoi(id)
	if err != nil {
		return fmt.Errorf("Invalid ID format")
	}

	if idInt < 0 {
		userID := -idInt
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()

		var status string
		var role string
		err = tx.QueryRow(`SELECT status, role FROM users WHERE id = ? FOR UPDATE`, userID).Scan(&status, &role)
		if err != nil {
			return fmt.Errorf("User tidak ditemukan")
		}
		if role != "teacher" || status != "pending" {
			return fmt.Errorf("User bukan guru pending")
		}

		if _, err := tx.Exec(`UPDATE users SET status = 'inactive', updated_at = NOW() WHERE id = ?`, userID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE teacher_profiles SET active = 0, updated_at = NOW() WHERE user_id = ?`, userID); err != nil {
			return err
		}

		if err := tx.Commit(); err != nil {
			return err
		}
		utils.LogActivity(
			s.db,
			adminUserID,
			"reject_teacher_registration",
			fmt.Sprintf("Admin menolak pendaftaran guru user_id=%d", userID),
			ip,
			userAgent,
		)
		return nil
	}

	_, err = s.db.Exec(
		`UPDATE teacher_roles SET status = 'rejected', verified_by = ?, verified_at = NOW(), updated_at = NOW() WHERE id = ?`,
		adminUserID, id,
	)
	if err != nil {
		return err
	}
	utils.LogActivity(
		s.db,
		adminUserID,
		"reject_teacher_role",
		fmt.Sprintf("Admin menolak permintaan peran id=%s", id),
		ip,
		userAgent,
	)
	return nil
}

func (s *adminService) GetAcademicYears() ([]domain.AcademicYear, error) {
	rows, err := s.db.Query(`SELECT id, name, is_active, start_date, end_date FROM academic_years ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.AcademicYear
	for rows.Next() {
		var a domain.AcademicYear
		if err := rows.Scan(&a.ID, &a.Name, &a.IsActive, &a.StartDate, &a.EndDate); err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, nil
}

func (s *adminService) CreateAcademicYear(name, startDate, endDate string, adminUserID int64, ip, userAgent string) error {
	_, err := s.db.Exec(`INSERT INTO academic_years (name, start_date, end_date, is_active) VALUES (?, ?, ?, 0)`,
		name, startDate, endDate)
	if err != nil {
		return err
	}
	utils.LogActivity(s.db, adminUserID, "create_academic_year", "Admin membuat tahun ajaran: "+name, ip, userAgent)
	return nil
}

func (s *adminService) UpdateAcademicYear(id string, name *string, isActive *bool, startDate, endDate *string, adminUserID int64, ip, userAgent string) error {
	if isActive != nil && *isActive {
		s.db.Exec(`UPDATE academic_years SET is_active = 0`)
	}
	s.db.Exec(`UPDATE academic_years SET name = COALESCE(?, name), is_active = COALESCE(?, is_active), start_date = COALESCE(?, start_date), end_date = COALESCE(?, end_date) WHERE id = ?`,
		name, isActive, startDate, endDate, id)
	utils.LogActivity(s.db, adminUserID, "update_academic_year", "Admin memperbarui tahun ajaran id="+id, ip, userAgent)
	return nil
}

func (s *adminService) DeleteAcademicYear(id string, adminUserID int64, ip, userAgent string) error {
	res, err := s.db.Exec(`DELETE FROM academic_years WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	utils.LogActivity(
		s.db,
		adminUserID,
		"delete_academic_year",
		fmt.Sprintf("Admin menghapus tahun ajaran id=%s (rows_affected=%d)", id, affected),
		ip,
		userAgent,
	)
	return nil
}

func (s *adminService) GetClasses() ([]domain.AdminClassItem, error) {
	rows, err := s.db.Query(`SELECT c.id, c.class_name, c.major_id, c.grade_level, COALESCE(m.name,'') FROM classes c LEFT JOIN majors m ON m.id = c.major_id ORDER BY c.class_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.AdminClassItem
	for rows.Next() {
		var item domain.AdminClassItem
		if err := rows.Scan(&item.ID, &item.ClassName, &item.MajorID, &item.GradeLevel, &item.MajorName); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *adminService) CreateClass(className string, majorID int, adminUserID int64, ip, userAgent string) error {
	s.db.Exec(`INSERT INTO classes (class_name, major_id) VALUES (?, ?)`, className, majorID)
	utils.LogActivity(s.db, adminUserID, "create_class", "Admin membuat kelas: "+className, ip, userAgent)
	return nil
}

func (s *adminService) UpdateClass(id, className string, majorID int, adminUserID int64, ip, userAgent string) error {
	s.db.Exec(`UPDATE classes SET class_name = ?, major_id = ? WHERE id = ?`, className, majorID, id)
	utils.LogActivity(s.db, adminUserID, "update_class", "Admin memperbarui kelas id="+id, ip, userAgent)
	return nil
}

func (s *adminService) DeleteClass(id string, adminUserID int64, ip, userAgent string) error {
	res, err := s.db.Exec(`DELETE FROM classes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	utils.LogActivity(
		s.db,
		adminUserID,
		"delete_class",
		fmt.Sprintf("Admin menghapus kelas id=%s (rows_affected=%d)", id, affected),
		ip,
		userAgent,
	)
	return nil
}

func (s *adminService) GetMajors() ([]domain.AdminMajorItem, error) {
	rows, err := s.db.Query(`SELECT id, name, code FROM majors ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.AdminMajorItem
	for rows.Next() {
		var item domain.AdminMajorItem
		if err := rows.Scan(&item.ID, &item.MajorName, &item.MajorShort); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *adminService) CreateMajor(name, short string, adminUserID int64, ip, userAgent string) error {
	_, err := s.db.Exec(`INSERT INTO majors (name, code) VALUES (?, ?)`, name, short)
	if err != nil {
		return err
	}
	utils.LogActivity(s.db, adminUserID, "create_major", "Admin membuat jurusan: "+name, ip, userAgent)
	return nil
}

func (s *adminService) UpdateMajor(id, name, short string, adminUserID int64, ip, userAgent string) error {
	_, err := s.db.Exec(`UPDATE majors SET name = ?, code = ? WHERE id = ?`, name, short, id)
	if err != nil {
		return err
	}
	utils.LogActivity(s.db, adminUserID, "update_major", "Admin memperbarui jurusan id="+id, ip, userAgent)
	return nil
}

func (s *adminService) DeleteMajor(id string, adminUserID int64, ip, userAgent string) error {
	res, err := s.db.Exec(`DELETE FROM majors WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	utils.LogActivity(
		s.db,
		adminUserID,
		"delete_major",
		fmt.Sprintf("Admin menghapus jurusan id=%s (rows_affected=%d)", id, affected),
		ip,
		userAgent,
	)
	return nil
}

func (s *adminService) GetSubjects() ([]domain.AdminSubjectItem, error) {
	rows, err := s.db.Query(`SELECT id, name, code FROM subjects ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.AdminSubjectItem
	for rows.Next() {
		var item domain.AdminSubjectItem
		if err := rows.Scan(&item.ID, &item.SubjectName, &item.SubjectCode); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *adminService) CreateSubject(name, code string, adminUserID int64, ip, userAgent string) error {
	_, err := s.db.Exec(`INSERT INTO subjects (name, code) VALUES (?, ?)`, name, code)
	if err != nil {
		return err
	}
	utils.LogActivity(s.db, adminUserID, "create_subject", "Admin membuat mata pelajaran: "+name, ip, userAgent)
	return nil
}

func (s *adminService) UpdateSubject(id, name, code string, adminUserID int64, ip, userAgent string) error {
	_, err := s.db.Exec(`UPDATE subjects SET name = ?, code = ? WHERE id = ?`, name, code, id)
	if err != nil {
		return err
	}
	utils.LogActivity(s.db, adminUserID, "update_subject", "Admin memperbarui mata pelajaran id="+id, ip, userAgent)
	return nil
}

func (s *adminService) DeleteSubject(id string, adminUserID int64, ip, userAgent string) error {
	res, err := s.db.Exec(`DELETE FROM subjects WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	utils.LogActivity(
		s.db,
		adminUserID,
		"delete_subject",
		fmt.Sprintf("Admin menghapus mata pelajaran id=%s (rows_affected=%d)", id, affected),
		ip,
		userAgent,
	)
	return nil
}

func (s *adminService) GetEnrollments(classID, search string, page, limit int) ([]domain.AdminEnrollmentItem, int, int, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	baseWhere := "sce.is_active = 1"
	args := []any{}
	if classID != "" {
		baseWhere += " AND sce.class_id = ?"
		args = append(args, classID)
	}
	if search != "" {
		baseWhere += " AND (sp.full_name LIKE ? OR sp.student_code LIKE ?)"
		s := "%" + search + "%"
		args = append(args, s, s)
	}

	countSQL := `SELECT COUNT(*) FROM student_class_enrollments sce JOIN student_profiles sp ON sp.id = sce.student_id WHERE ` + baseWhere
	var total int
	if err := s.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, 0, 0, 0, err
	}

	listSQL := `SELECT sce.id, sce.student_id, sce.class_id, sce.academic_year_id, COALESCE(sp.full_name,'') as student_name, COALESCE(sp.student_code,'') as student_code, sce.promotion_note
		FROM student_class_enrollments sce
		JOIN student_profiles sp ON sp.id = sce.student_id
		WHERE ` + baseWhere + ` ORDER BY sp.full_name LIMIT ? OFFSET ?`
	listArgs := append(append([]any{}, args...), limit, offset)

	rows, err := s.db.Query(listSQL, listArgs...)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	defer rows.Close()

	var items []domain.AdminEnrollmentItem
	for rows.Next() {
		var item domain.AdminEnrollmentItem
		if err := rows.Scan(&item.ID, &item.StudentID, &item.ClassID, &item.AcademicYearID, &item.StudentName, &item.StudentCode, &item.Notes); err != nil {
			return nil, 0, 0, 0, err
		}
		items = append(items, item)
	}
	totalPages := (total + limit - 1) / limit
	return items, total, totalPages, page, nil
}

func (s *adminService) CreateEnrollment(studentID, classID, academicYearID int, notes *string, adminUserID int64, ip, userAgent string) error {
	if academicYearID == 0 {
		err := s.db.QueryRow(`SELECT id FROM academic_years WHERE is_active = 1 LIMIT 1`).Scan(&academicYearID)
		if err != nil {
			return fmt.Errorf("Tidak ada tahun ajaran aktif. Silakan aktifkan tahun ajaran terlebih dahulu.")
		}
	}

	var notePtr *string
	if notes != nil {
		trimmed := strings.TrimSpace(*notes)
		if trimmed != "" {
			notePtr = &trimmed
		}
	}

	var existingID int
	err := s.db.QueryRow(
		`SELECT id FROM student_class_enrollments WHERE student_id = ? AND academic_year_id = ? LIMIT 1`,
		studentID, academicYearID,
	).Scan(&existingID)
	if err == nil {
		return fmt.Errorf("Siswa sudah terdaftar di tahun ajaran ini.")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("Gagal memeriksa data enrollment. Silakan coba lagi.")
	}

	_, err = s.db.Exec(
		`INSERT INTO student_class_enrollments (student_id, class_id, academic_year_id, is_active, promotion_note) VALUES (?, ?, ?, 1, ?)`,
		studentID, classID, academicYearID, notePtr,
	)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) {
			switch mysqlErr.Number {
			case 1062:
				return fmt.Errorf("Siswa sudah terdaftar di tahun ajaran ini.")
			case 1452:
				return fmt.Errorf("Data siswa, kelas, atau tahun ajaran tidak valid.")
			default:
				return fmt.Errorf("Gagal mendaftarkan siswa. Silakan coba lagi.")
			}
		}
		return fmt.Errorf("Gagal mendaftarkan siswa. Silakan coba lagi.")
	}

	noteSummary := ""
	if notePtr != nil {
		noteSummary = *notePtr
	}
	utils.LogActivity(
		s.db,
		adminUserID,
		"create_enrollment",
		fmt.Sprintf("Admin mendaftarkan student_id=%d ke class_id=%d untuk academic_year_id=%d. Catatan: %q", studentID, classID, academicYearID, noteSummary),
		ip,
		userAgent,
	)
	return nil
}

func (s *adminService) DeleteEnrollment(id string, adminUserID int64, ip, userAgent string) error {
	res, err := s.db.Exec(`DELETE FROM student_class_enrollments WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	utils.LogActivity(
		s.db,
		adminUserID,
		"delete_enrollment",
		fmt.Sprintf("Admin menghapus enrollment id=%s (rows_affected=%d)", id, affected),
		ip,
		userAgent,
	)
	return nil
}

func (s *adminService) GetSchoolConfig() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT config_key, config_value FROM school_config`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	config := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		config[k] = v
	}
	return config, nil
}

func (s *adminService) GetPrincipalConfig() (fullName, signatureURL string) {
	err := s.db.QueryRow(`
		SELECT COALESCE(pp.full_name, ''), COALESCE(pp.signature_url, '')
		FROM principal_profiles pp
		INNER JOIN users u ON u.id = pp.user_id
		WHERE u.role = 'kepala_sekolah'
		  AND u.deleted_at IS NULL
		  AND u.status = 'active'
		  AND pp.active = 1
		ORDER BY pp.updated_at DESC
		LIMIT 1
	`).Scan(&fullName, &signatureURL)
	if err != nil {
		return "", ""
	}
	return fullName, signatureURL
}

func (s *adminService) UpdateSchoolConfig(values map[string]string, adminUserID int64, ip, userAgent string) error {
	for k, v := range values {
		s.db.Exec(`INSERT INTO school_config (config_key, config_value) VALUES (?, ?) ON DUPLICATE KEY UPDATE config_value = ?`, k, v, v)
	}
	utils.LogActivity(s.db, adminUserID, "update_school_config", "Admin memperbarui konfigurasi sekolah", ip, userAgent)
	return nil
}

func (s *adminService) UploadConfigImage(configKey, filePath string, adminUserID int64, ip, userAgent string) (string, error) {
	_, dbErr := s.db.Exec(`INSERT INTO school_config (config_key, config_value) VALUES (?, ?) ON DUPLICATE KEY UPDATE config_value = ?`, configKey, filePath, filePath)
	if dbErr != nil {
		return "", fmt.Errorf("Gagal menyimpan konfigurasi ke database: %s", dbErr.Error())
	}

	utils.LogActivity(s.db, adminUserID, "upload_config_image", "Admin mengunggah gambar: "+configKey, ip, userAgent)
	return filePath, nil
}

func (s *adminService) GetAuditLogs(activityType, search string, page, limit int) ([]domain.AuditLogItem, int, int, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 50
	}
	offset := (page - 1) * limit

	var (
		total        int
		args         []any
		whereClauses []string
	)
	if activityType != "" {
		whereClauses = append(whereClauses, "activity_type = ?")
		args = append(args, activityType)
	}
	if search != "" {
		whereClauses = append(whereClauses, "(description LIKE ? OR CAST(user_id AS CHAR) LIKE ?)")
		s := "%" + search + "%"
		args = append(args, s, s)
	}
	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	countQuery := "SELECT COUNT(*) FROM activity_logs" + whereSQL
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, 0, 0, err
	}

	listArgs := append(append([]any{}, args...), limit, offset)
	rows, err := s.db.Query(`
		SELECT id, user_id, activity_type AS action, description AS details, ip_address, user_agent, created_at
		FROM activity_logs`+whereSQL+`
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, listArgs...)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	defer rows.Close()

	wib := time.FixedZone("WIB", 7*60*60)
	logs := []domain.AuditLogItem{}
	for rows.Next() {
		var l domain.AuditLogItem
		var createdAt time.Time
		if err := rows.Scan(&l.ID, &l.UserID, &l.Action, &l.Details, &l.IPAddress, &l.UserAgent, &createdAt); err != nil {
			return nil, 0, 0, 0, err
		}
		createdAt = createdAt.In(wib)
		l.CreatedAt = createdAt.Format(time.RFC3339)
		logs = append(logs, l)
	}
	totalPages := (total + limit - 1) / limit
	return logs, total, totalPages, page, nil
}
