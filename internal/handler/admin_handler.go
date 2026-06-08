package handler

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Refliqx/backend-eletter/internal/response"
	"github.com/Refliqx/backend-eletter/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
)

type AdminHandler struct {
	db *sql.DB
}

func NewAdminHandler(db *sql.DB) *AdminHandler {
	return &AdminHandler{db: db}
}

func (h *AdminHandler) GetStats(c *gin.Context) {
	stats := map[string]int{}
	rows := []struct{ key, query string }{
		{"total_students", `SELECT COUNT(*) FROM users WHERE role='student' AND deleted_at IS NULL`},
		{"total_teachers", `SELECT COUNT(*) FROM users WHERE role IN ('teacher','kepala_sekolah') AND deleted_at IS NULL`},
		{"pending_requests", `SELECT COUNT(*) FROM requests WHERE status='pending'`},
		{"active_tokens", `SELECT COUNT(*) FROM registration_tokens WHERE used_count < usage_limit AND (expires_at IS NULL OR expires_at > NOW())`},
	}
	for _, r := range rows {
		var count int
		_ = h.db.QueryRow(r.query).Scan(&count)
		stats[r.key] = count
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": stats})
}

func (h *AdminHandler) GetUsers(c *gin.Context) {
	role := c.Query("role")
	status := c.Query("status")
	search := c.Query("search")

	query := `SELECT u.id, u.email, u.role, u.status, COALESCE(tp.full_name, sp.full_name, ap.full_name, pp.full_name, '') as full_name
		FROM users u 
		LEFT JOIN teacher_profiles tp ON tp.user_id = u.id 
		LEFT JOIN student_profiles sp ON sp.user_id = u.id
		LEFT JOIN admin_profiles ap ON ap.user_id = u.id
		LEFT JOIN principal_profiles pp ON pp.user_id = u.id
		WHERE u.deleted_at IS NULL`
	args := []any{}

	if role != "" {
		query += " AND u.role = ?"
		args = append(args, role)
	}
	if status != "" {
		query += " AND u.status = ?"
		args = append(args, status)
	}
	if search != "" {
		query += " AND (u.email LIKE ? OR tp.full_name LIKE ? OR sp.full_name LIKE ? OR ap.full_name LIKE ? OR pp.full_name LIKE ?)"
		s := "%" + search + "%"
		args = append(args, s, s, s, s, s)
	}
	query += " ORDER BY u.id DESC LIMIT 100"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type User struct {
		ID       int     `json:"id"`
		Email    *string `json:"email"`
		Role     string  `json:"role"`
		Status   string  `json:"status"`
		FullName string  `json:"full_name"`
	}
	var users []User
	for rows.Next() {
		var u User
		rows.Scan(&u.ID, &u.Email, &u.Role, &u.Status, &u.FullName)
		users = append(users, u)
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": users})
}

func (h *AdminHandler) UpdateUserStatus(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	_, err := h.db.Exec(`UPDATE users SET status = ? WHERE id = ?`, body.Status, id)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	adminUserID := toIntFromContext(c, "userId")
	utils.LogActivity(
		h.db,
		int64(adminUserID),
		"update_user_status",
		fmt.Sprintf("Admin mengubah status user id=%s menjadi '%s'", id, body.Status),
		c.ClientIP(),
		c.GetHeader("User-Agent"),
	)
	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Status berhasil diperbarui"})
}

// knownRoles mirrors the canonical role values used across the system.
// Keep in sync with DB role_name values.
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
	}
	return ""
}

func roleProfileHasDeletedAt(role string) bool {
	return role == "student" || role == "teacher"
}

// UpdateUser allows an admin to change a user's role and/or full_name.
// Body: {"role": "student|teacher|kepala_sekolah|admin", "full_name": "..."} - both optional.
// When the role changes, the existing profile (if any) for the old role is
// soft-deleted and a new profile row is created for the new role with the
// provided (or existing) full_name. If the new role is unchanged but a
// full_name is supplied, the matching profile row's full_name is updated.
func (h *AdminHandler) UpdateUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, http.StatusBadRequest, "ID tidak valid")
		return
	}
	var body struct {
		Role     *string `json:"role"`
		FullName *string `json:"full_name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.Role == nil && body.FullName == nil {
		response.Error(c, http.StatusBadRequest, "Tidak ada perubahan yang dikirim")
		return
	}
	if body.Role != nil && !knownRoles[*body.Role] {
		response.Error(c, http.StatusBadRequest, "Role tidak dikenal: "+*body.Role)
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	// Lock the user row.
	var currentRole string
	var currentStatus string
	err = tx.QueryRow(`SELECT role, status FROM users WHERE id = ? AND deleted_at IS NULL FOR UPDATE`, id).Scan(&currentRole, &currentStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.Error(c, http.StatusNotFound, "Pengguna tidak ditemukan")
			return
		}
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	targetRole := currentRole
	if body.Role != nil {
		targetRole = *body.Role
	}

	// Update the canonical users table.
	if body.Role != nil {
		if _, err := tx.Exec(`UPDATE users SET role = ?, updated_at = NOW() WHERE id = ?`, *body.Role, id); err != nil {
			response.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Resolve the full_name to apply: explicit body value wins, else keep existing.
	newFullName := ""
	if body.FullName != nil {
		newFullName = strings.TrimSpace(*body.FullName)
	}
	if newFullName == "" {
		_ = tx.QueryRow(
			fmt.Sprintf(`SELECT full_name FROM %s WHERE user_id = ? ORDER BY id DESC LIMIT 1`, roleProfileTable(currentRole)),
			id,
		).Scan(&newFullName)
	}

	// Soft-delete the old role's profile (if it has a profile table).
	oldTable := roleProfileTable(currentRole)
	if oldTable != "" && currentRole != targetRole {
		if roleProfileHasDeletedAt(currentRole) {
			if _, err := tx.Exec(
				fmt.Sprintf(`UPDATE %s SET deleted_at = NOW(), active = 0, updated_at = NOW() WHERE user_id = ? AND deleted_at IS NULL`, oldTable),
				id,
			); err != nil {
				response.Error(c, http.StatusInternalServerError, fmt.Sprintf("Gagal menonaktifkan profil lama: %s", err.Error()))
				return
			}
		} else {
			// admin_profiles and principal_profiles have no deleted_at; just leave them
			// (admin may keep a parallel principal profile record even after role switch).
		}
	}

	// Update or insert profile for the target role.
	newTable := roleProfileTable(targetRole)
	if newTable != "" {
		// If the same table for the same user exists and is active, update full_name.
		if roleProfileHasDeletedAt(targetRole) {
			res, err := tx.Exec(
				fmt.Sprintf(`UPDATE %s SET full_name = ?, updated_at = NOW() WHERE user_id = ? AND deleted_at IS NULL`, newTable),
				newFullName, id,
			)
			if err != nil {
				response.Error(c, http.StatusInternalServerError, err.Error())
				return
			}
			affected, _ := res.RowsAffected()
			if affected == 0 {
				// No active profile: insert a fresh one. Default required columns to safe placeholders.
				switch targetRole {
				case "student":
					if _, err := tx.Exec(
						`INSERT INTO student_profiles (user_id, full_name, gender, active) VALUES (?, ?, 'other', 1)`,
						id, newFullName,
					); err != nil {
						response.Error(c, http.StatusInternalServerError, fmt.Sprintf("Gagal membuat profil siswa: %s", err.Error()))
						return
					}
				case "teacher":
					if _, err := tx.Exec(
						`INSERT INTO teacher_profiles (user_id, full_name, active) VALUES (?, ?, 1)`,
						id, newFullName,
					); err != nil {
						response.Error(c, http.StatusInternalServerError, fmt.Sprintf("Gagal membuat profil guru: %s", err.Error()))
						return
					}
				}
			}
		} else {
			// admin / principal - update the most recent record; insert if none.
			res, err := tx.Exec(
				fmt.Sprintf(`UPDATE %s SET full_name = ?, updated_at = NOW() WHERE user_id = ?`, newTable),
				newFullName, id,
			)
			if err != nil {
				response.Error(c, http.StatusInternalServerError, err.Error())
				return
			}
			affected, _ := res.RowsAffected()
			if affected == 0 {
				switch targetRole {
				case "admin":
					if _, err := tx.Exec(
						`INSERT INTO admin_profiles (user_id, full_name) VALUES (?, ?)`,
						id, newFullName,
					); err != nil {
						response.Error(c, http.StatusInternalServerError, fmt.Sprintf("Gagal membuat profil admin: %s", err.Error()))
						return
					}
				case "kepala_sekolah":
					if _, err := tx.Exec(
						`INSERT INTO principal_profiles (user_id, full_name, active) VALUES (?, ?, 1)`,
						id, newFullName,
					); err != nil {
						response.Error(c, http.StatusInternalServerError, fmt.Sprintf("Gagal membuat profil kepala sekolah: %s", err.Error()))
						return
					}
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	adminUserID := toIntFromContext(c, "userId")
	descParts := []string{fmt.Sprintf("Admin memperbarui user id=%d", id)}
	if body.Role != nil {
		descParts = append(descParts, fmt.Sprintf("role: %s -> %s", currentRole, targetRole))
	}
	if body.FullName != nil {
		descParts = append(descParts, fmt.Sprintf("nama: '%s'", *body.FullName))
	}
	utils.LogActivity(
		h.db,
		int64(adminUserID),
		"update_user",
		strings.Join(descParts, "; "),
		c.ClientIP(),
		c.GetHeader("User-Agent"),
	)

	response.Raw(c, http.StatusOK, gin.H{
		"success": true,
		"message": "Pengguna berhasil diperbarui",
		"data": gin.H{
			"id":        id,
			"role":      targetRole,
			"full_name": newFullName,
			"status":    currentStatus,
		},
	})
}

// CreateUser lets an admin provision a new account directly (no token required).
// Useful for the "Tambah Pengguna" UI on /admin/pengguna where the admin needs
// to create a user without going through the public registration flow.
func (h *AdminHandler) CreateUser(c *gin.Context) {
	var body struct {
		FullName string `json:"full_name" binding:"required"`
		Email    string `json:"email" binding:"required,email"`
		Role     string `json:"role" binding:"required"`
		Status   string `json:"status"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	role := strings.ToLower(strings.TrimSpace(body.Role))
	if !knownRoles[role] {
		response.Error(c, http.StatusBadRequest, "Peran tidak dikenal")
		return
	}

	// Username-based roles (admin, kepala_sekolah, tu) get an auto-generated
	// username like ADM-001, KS-001, TU-001 and set email to NULL.
	// Email-based roles (student, teacher) keep the email and set username to NULL.
	usernameRoles := map[string]string{
		"admin":          "ADM",
		"kepala_sekolah": "KS",
		"tu":             "TU",
	}
	prefix, usesUsername := usernameRoles[role]

	var username *string
	var email *string
	rawEmail := strings.ToLower(strings.TrimSpace(body.Email))

	if usesUsername {
		// Generate sequential username.
		var lastNum int
		err := h.db.QueryRow(
			`SELECT COALESCE(MAX(CAST(SUBSTRING_INDEX(username, '-', -1) AS UNSIGNED)), 0) FROM users WHERE username LIKE ? AND deleted_at IS NULL`,
			prefix+"-%",
		).Scan(&lastNum)
		if err != nil {
			response.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		generated := fmt.Sprintf("%s-%03d", prefix, lastNum+1)
		username = &generated
		email = nil
	} else {
		// Email-based roles.
		email = &rawEmail
		username = nil

		// Reject duplicates up-front so the user gets a clear message.
		var existingID int
		err := h.db.QueryRow(`SELECT id FROM users WHERE email = ? AND deleted_at IS NULL`, rawEmail).Scan(&existingID)
		if err == nil {
			response.Error(c, http.StatusConflict, "Email sudah terdaftar")
			return
		}
		if err != nil && err != sql.ErrNoRows {
			response.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
	}

	status := strings.TrimSpace(body.Status)
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "pending" && status != "inactive" {
		response.Error(c, http.StatusBadRequest, "Status tidak valid")
		return
	}

	password := body.Password
	if password == "" {
		password = "e-letter-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	hash, err := utils.HashPassword(password)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal memproses kata sandi")
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`INSERT INTO users (username, email, password_hash, role, status) VALUES (?, ?, ?, ?, ?)`,
		username, email, hash, role, status,
	)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			identifier := "Email"
			if usesUsername {
				identifier = "Username"
			}
			response.Error(c, http.StatusConflict, identifier+" sudah terdaftar")
			return
		}
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	userID, _ := res.LastInsertId()

	profileTable := roleProfileTable(role)
	if profileTable == "" {
		response.Error(c, http.StatusBadRequest, "Peran tidak memiliki profil")
		return
	}

	active := 0
	if status == "active" {
		active = 1
	}

	switch role {
	case "student":
		_, err = tx.Exec(
			`INSERT INTO student_profiles (user_id, full_name, gender, active) VALUES (?, ?, 'other', ?)`,
			userID, body.FullName, active,
		)
	case "teacher":
		_, err = tx.Exec(
			`INSERT INTO teacher_profiles (user_id, full_name, active) VALUES (?, ?, ?)`,
			userID, body.FullName, active,
		)
	case "kepala_sekolah":
		_, err = tx.Exec(
			`INSERT INTO principal_profiles (user_id, full_name, active) VALUES (?, ?, ?)`,
			userID, body.FullName, active,
		)
	case "admin":
		_, err = tx.Exec(
			`INSERT INTO admin_profiles (user_id, full_name) VALUES (?, ?)`,
			userID, body.FullName,
		)
	case "tu":
		_, err = tx.Exec(
			`INSERT INTO tu_profiles (user_id, full_name) VALUES (?, ?)`,
			userID, body.FullName,
		)
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal membuat profil: "+err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	identifier := rawEmail
	if usesUsername {
		identifier = *username
	}

	adminUserID := toIntFromContext(c, "userId")
	utils.LogActivity(
		h.db,
		int64(adminUserID),
		"create_user",
		fmt.Sprintf("Admin membuat akun baru: %s (%s) dengan role=%s, status=%s", body.FullName, identifier, role, status),
		c.ClientIP(),
		c.GetHeader("User-Agent"),
	)
	response.Raw(c, http.StatusCreated, gin.H{
		"success": true,
		"message": "Pengguna berhasil dibuat",
		"data": gin.H{
			"id":        userID,
			"username":  username,
			"email":     email,
			"role":      role,
			"full_name": body.FullName,
			"status":    status,
			"password":  password,
		},
	})
}

// AdminDeleteLetter force-deletes any permission request, regardless of
// ownership. Soft-delete via requests.deleted_at; only admins may call it.
func (h *AdminHandler) AdminDeleteLetter(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		response.Error(c, http.StatusBadRequest, "ID tidak valid")
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	// Lock and read metadata for audit + cascade decisions.
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
			response.Error(c, http.StatusNotFound, "Surat tidak ditemukan atau sudah dihapus")
			return
		}
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	if _, err := tx.Exec(`UPDATE requests SET deleted_at = NOW() WHERE id = ?`, id); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	adminUserID := toIntFromContext(c, "userId")
	number := "-"
	if requestNumber.Valid {
		number = requestNumber.String
	}
	utils.LogActivity(
		h.db,
		int64(adminUserID),
		"admin_delete_letter",
		fmt.Sprintf("Admin menghapus surat id=%d nomor=%s atas permintaan user_id=%d (status=%s)", id, number, requesterUserID, status.String),
		c.ClientIP(),
		c.GetHeader("User-Agent"),
	)

	// Best-effort SSE refresh so the affected user's UI updates.
	// (notification_publisher would normally be injected; admin handler has
	// access to the raw db only. We rely on clients polling or a 5s refresh.)

	response.Raw(c, http.StatusOK, gin.H{
		"success": true,
		"message": "Surat berhasil dihapus",
		"data": gin.H{
			"id":                id,
			"letter_type":       letterType.String,
			"request_number":    number,
			"requester_user_id": requesterUserID,
		},
	})
}

func (h *AdminHandler) GetRegistrationTokens(c *gin.Context) {
	rows, err := h.db.Query(`SELECT token_id, token, role_id, usage_limit, used_count, expires_at, created_at FROM registration_tokens ORDER BY created_at DESC`)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type Token struct {
		ID         int     `json:"id"`
		Token      string  `json:"token"`
		RoleID     int     `json:"role_id"`
		UsageLimit int     `json:"usage_limit"`
		UsedCount  int     `json:"used_count"`
		ExpiresAt  *string `json:"expires_at"`
		CreatedAt  string  `json:"created_at"`
	}
	var tokens []Token
	for rows.Next() {
		var t Token
		rows.Scan(&t.ID, &t.Token, &t.RoleID, &t.UsageLimit, &t.UsedCount, &t.ExpiresAt, &t.CreatedAt)
		tokens = append(tokens, t)
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": tokens})
}

func (h *AdminHandler) CreateRegistrationToken(c *gin.Context) {
	var body struct {
		Token      string  `json:"token" binding:"required"`
		RoleID     int     `json:"role_id" binding:"required"`
		UsageLimit int     `json:"usage_limit"`
		ExpiresAt  *string `json:"expires_at"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.UsageLimit == 0 {
		body.UsageLimit = 100
	}
	_, err := h.db.Exec(`INSERT INTO registration_tokens (token, role_id, usage_limit, expires_at) VALUES (?, ?, ?, ?)`,
		body.Token, body.RoleID, body.UsageLimit, body.ExpiresAt)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Raw(c, http.StatusCreated, gin.H{"success": true, "message": "Token berhasil dibuat"})
}

func (h *AdminHandler) DeleteRegistrationToken(c *gin.Context) {
	id := c.Param("id")
	_, err := h.db.Exec(`DELETE FROM registration_tokens WHERE token_id = ?`, id)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Token berhasil dihapus"})
}

func (h *AdminHandler) VerifyTeacherRole(c *gin.Context) {
	id := c.Param("id")
	adminUserID := toIntFromContext(c, "userId")

	idInt, err := strconv.Atoi(id)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid ID format")
		return
	}

	if idInt < 0 {
		userID := -idInt
		tx, err := h.db.Begin()
		if err != nil {
			response.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer tx.Rollback()

		var status string
		var role string
		err = tx.QueryRow(`SELECT status, role FROM users WHERE id = ? FOR UPDATE`, userID).Scan(&status, &role)
		if err != nil {
			response.Error(c, http.StatusNotFound, "User tidak ditemukan")
			return
		}
		if role != "teacher" || status != "pending" {
			response.Error(c, http.StatusBadRequest, "User bukan guru pending")
			return
		}

		if _, err := tx.Exec(`UPDATE users SET status = 'active', updated_at = NOW() WHERE id = ?`, userID); err != nil {
			response.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		if _, err := tx.Exec(`UPDATE teacher_profiles SET active = 1, updated_at = NOW() WHERE user_id = ?`, userID); err != nil {
			response.Error(c, http.StatusInternalServerError, err.Error())
			return
		}

		if err := tx.Commit(); err != nil {
			response.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		utils.LogActivity(
			h.db,
			int64(adminUserID),
			"approve_teacher_registration",
			fmt.Sprintf("Admin menyetujui pendaftaran guru user_id=%d", userID),
			c.ClientIP(),
			c.GetHeader("User-Agent"),
		)
		response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Akun guru berhasil disetujui"})
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	// Lock the row and read metadata needed for assignment writes.
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
		response.Error(c, http.StatusNotFound, "Role tidak ditemukan")
		return
	}

	// Activate the role.
	if _, err := tx.Exec(
		`UPDATE teacher_roles SET status = 'active', verified_at = NOW(), verified_by = ? WHERE id = ?`,
		adminUserID, id,
	); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Write to the canonical assignment table based on role type.
	switch roleName {
	case "wali_kelas":
		if homeroomClassID.Valid {
			// Deactivate any existing assignment for this class first (one class, one wali).
			if _, err := tx.Exec(
				`UPDATE class_homeroom_assignments SET is_active = 0, updated_at = NOW()
				  WHERE class_id = ? AND academic_year_id = ?`,
				homeroomClassID.Int64, academicYearID,
			); err != nil {
				response.Error(c, http.StatusInternalServerError, err.Error())
				return
			}
			if _, err := tx.Exec(
				`INSERT INTO class_homeroom_assignments (class_id, teacher_id, academic_year_id, is_active)
				 VALUES (?, ?, ?, 1)
				 ON DUPLICATE KEY UPDATE teacher_id = VALUES(teacher_id), is_active = 1, updated_at = NOW()`,
				homeroomClassID.Int64, teacherID, academicYearID,
			); err != nil {
				response.Error(c, http.StatusInternalServerError, err.Error())
				return
			}
		}

	case "kapro":
		if majorID.Valid {
			// Deactivate existing kapro for this major.
			if _, err := tx.Exec(
				`UPDATE major_head_assignments SET is_active = 0, updated_at = NOW()
				  WHERE major_id = ? AND academic_year_id = ?`,
				majorID.Int64, academicYearID,
			); err != nil {
				response.Error(c, http.StatusInternalServerError, err.Error())
				return
			}
			if _, err := tx.Exec(
				`INSERT INTO major_head_assignments (major_id, teacher_id, academic_year_id, is_active)
				 VALUES (?, ?, ?, 1)
				 ON DUPLICATE KEY UPDATE teacher_id = VALUES(teacher_id), is_active = 1, updated_at = NOW()`,
				majorID.Int64, teacherID, academicYearID,
			); err != nil {
				response.Error(c, http.StatusInternalServerError, err.Error())
				return
			}
		}

	case "guru_mapel":
		if subjectIDsRaw.Valid && subjectIDsRaw.String != "" {
			// Get all active classes for this academic year to create schedules.
			rows, err := tx.Query(
				`SELECT id FROM classes WHERE academic_year_id = ? AND is_active = 1`,
				academicYearID,
			)
			if err != nil {
				response.Error(c, http.StatusInternalServerError, err.Error())
				return
			}
			var classIDs []int64
			for rows.Next() {
				var cid int64
				_ = rows.Scan(&cid)
				classIDs = append(classIDs, cid)
			}
			rows.Close()

			// Parse comma-separated subject IDs.
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
					// Use senin as default day; the schedule can be updated by admin later.
					// start_time/end_time default to school start (07:00 – 08:00).
					if _, err := tx.Exec(
						`INSERT IGNORE INTO schedules
							(academic_year_id, class_id, subject_id, teacher_id,
							 day_of_week, start_time, end_time, is_active)
						VALUES (?, ?, ?, ?, 'senin', '07:00:00', '08:00:00', 1)`,
						academicYearID, classID, subjectID, teacherID,
					); err != nil {
						response.Error(c, http.StatusInternalServerError,
							fmt.Sprintf("Gagal membuat jadwal untuk subject %d kelas %d: %s", subjectID, classID, err.Error()))
						return
					}
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	utils.LogActivity(
		h.db,
		int64(adminUserID),
		"verify_teacher_role",
		fmt.Sprintf("Admin memverifikasi peran guru: role_id=%s role_name=%s teacher_id=%d", id, roleName, teacherID),
		c.ClientIP(),
		c.GetHeader("User-Agent"),
	)
	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Peran guru berhasil diverifikasi dan penugasan dibuat"})
}

func (h *AdminHandler) ListPendingTeacherRoles(c *gin.Context) {
	status := c.DefaultQuery("status", "pending")
	rows, err := h.db.Query(`
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
		ORDER BY created_at DESC
	`, status, status)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type PendingRole struct {
		ID            int     `json:"id"`
		TeacherID     int     `json:"teacher_id"`
		TeacherUserID int     `json:"teacher_user_id"`
		TeacherName   string  `json:"teacher_name"`
		RoleName      string  `json:"role_name"`
		Status        string  `json:"status"`
		ClassID       *int64  `json:"homeroom_class_id,omitempty"`
		ClassName     *string `json:"homeroom_class,omitempty"`
		MajorID       *int64  `json:"major_id,omitempty"`
		MajorName     *string `json:"major_name,omitempty"`
		SubjectIDs    *string `json:"subject_ids,omitempty"`
		CreatedAt     string  `json:"created_at"`
	}

	var items []PendingRole
	for rows.Next() {
		var rec PendingRole
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
			response.Error(c, http.StatusInternalServerError, err.Error())
			return
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
		items = []PendingRole{}
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": items})
}

func (h *AdminHandler) RejectTeacherRole(c *gin.Context) {
	id := c.Param("id")
	adminUserID := toIntFromContext(c, "userId")

	idInt, err := strconv.Atoi(id)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid ID format")
		return
	}

	if idInt < 0 {
		userID := -idInt
		tx, err := h.db.Begin()
		if err != nil {
			response.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		defer tx.Rollback()

		var status string
		var role string
		err = tx.QueryRow(`SELECT status, role FROM users WHERE id = ? FOR UPDATE`, userID).Scan(&status, &role)
		if err != nil {
			response.Error(c, http.StatusNotFound, "User tidak ditemukan")
			return
		}
		if role != "teacher" || status != "pending" {
			response.Error(c, http.StatusBadRequest, "User bukan guru pending")
			return
		}

		// Set status of user to 'inactive' (rejected registration) and de-activate teacher profile, WITHOUT soft deleting
		if _, err := tx.Exec(`UPDATE users SET status = 'inactive', updated_at = NOW() WHERE id = ?`, userID); err != nil {
			response.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		if _, err := tx.Exec(`UPDATE teacher_profiles SET active = 0, updated_at = NOW() WHERE user_id = ?`, userID); err != nil {
			response.Error(c, http.StatusInternalServerError, err.Error())
			return
		}

		if err := tx.Commit(); err != nil {
			response.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		utils.LogActivity(
			h.db,
			int64(adminUserID),
			"reject_teacher_registration",
			fmt.Sprintf("Admin menolak pendaftaran guru user_id=%d", userID),
			c.ClientIP(),
			c.GetHeader("User-Agent"),
		)
		response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Pendaftaran guru berhasil ditolak"})
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body) // reason is optional
	_, err = h.db.Exec(
		`UPDATE teacher_roles SET status = 'rejected', verified_by = ?, verified_at = NOW(), updated_at = NOW() WHERE id = ?`,
		adminUserID, id,
	)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	utils.LogActivity(
		h.db,
		int64(adminUserID),
		"reject_teacher_role",
		fmt.Sprintf("Admin menolak permintaan peran id=%s", id),
		c.ClientIP(),
		c.GetHeader("User-Agent"),
	)
	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Permintaan peran ditolak"})
}

func (h *AdminHandler) GetAcademicYears(c *gin.Context) {
	rows, err := h.db.Query(`SELECT id, name, is_active, start_date, end_date FROM academic_years ORDER BY id DESC`)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	type AY struct {
		ID        int    `json:"id"`
		Name      string `json:"name"`
		IsActive  bool   `json:"is_active"`
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
	}
	var items []AY
	for rows.Next() {
		var a AY
		rows.Scan(&a.ID, &a.Name, &a.IsActive, &a.StartDate, &a.EndDate)
		items = append(items, a)
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": items})
}

func (h *AdminHandler) CreateAcademicYear(c *gin.Context) {
	var body struct {
		Name      string `json:"name" binding:"required"`
		StartDate string `json:"start_date" binding:"required"`
		EndDate   string `json:"end_date" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	_, err := h.db.Exec(`INSERT INTO academic_years (name, start_date, end_date, is_active) VALUES (?, ?, ?, 0)`,
		body.Name, body.StartDate, body.EndDate)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Raw(c, http.StatusCreated, gin.H{"success": true, "message": "Tahun ajaran berhasil dibuat"})
}

func (h *AdminHandler) UpdateAcademicYear(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Name      *string `json:"name"`
		IsActive  *bool   `json:"is_active"`
		StartDate *string `json:"start_date"`
		EndDate   *string `json:"end_date"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.IsActive != nil && *body.IsActive {
		h.db.Exec(`UPDATE academic_years SET is_active = 0`)
	}
	h.db.Exec(`UPDATE academic_years SET name = COALESCE(?, name), is_active = COALESCE(?, is_active), start_date = COALESCE(?, start_date), end_date = COALESCE(?, end_date) WHERE id = ?`,
		body.Name, body.IsActive, body.StartDate, body.EndDate, id)
	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Tahun ajaran berhasil diperbarui"})
}

func (h *AdminHandler) DeleteAcademicYear(c *gin.Context) {
	id := c.Param("id")
	res, err := h.db.Exec(`DELETE FROM academic_years WHERE id = ?`, id)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	affected, _ := res.RowsAffected()
	adminUserID := toIntFromContext(c, "userId")
	utils.LogActivity(
		h.db,
		int64(adminUserID),
		"delete_academic_year",
		fmt.Sprintf("Admin menghapus tahun ajaran id=%s (rows_affected=%d)", id, affected),
		c.ClientIP(),
		c.GetHeader("User-Agent"),
	)
	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Tahun ajaran berhasil dihapus"})
}

func (h *AdminHandler) GetClasses(c *gin.Context) {
	rows, err := h.db.Query(`SELECT c.id, c.class_name, c.major_id, COALESCE(m.name,'') FROM classes c LEFT JOIN majors m ON m.id = c.major_id ORDER BY c.class_name`)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	type Class struct {
		ID        int    `json:"id"`
		ClassName string `json:"class_name"`
		MajorID   int    `json:"major_id"`
		MajorName string `json:"major_name"`
	}
	var items []Class
	for rows.Next() {
		var item Class
		rows.Scan(&item.ID, &item.ClassName, &item.MajorID, &item.MajorName)
		items = append(items, item)
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": items})
}

func (h *AdminHandler) CreateClass(c *gin.Context) {
	var body struct {
		ClassName string `json:"class_name" binding:"required"`
		MajorID   int    `json:"major_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	h.db.Exec(`INSERT INTO classes (class_name, major_id) VALUES (?, ?)`, body.ClassName, body.MajorID)
	response.Raw(c, http.StatusCreated, gin.H{"success": true, "message": "Kelas berhasil dibuat"})
}

func (h *AdminHandler) UpdateClass(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		ClassName string `json:"class_name"`
		MajorID   int    `json:"major_id"`
	}
	c.ShouldBindJSON(&body)
	h.db.Exec(`UPDATE classes SET class_name = ?, major_id = ? WHERE id = ?`, body.ClassName, body.MajorID, id)
	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Kelas berhasil diperbarui"})
}

func (h *AdminHandler) DeleteClass(c *gin.Context) {
	classID := c.Param("id")
	res, err := h.db.Exec(`DELETE FROM classes WHERE id = ?`, classID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	affected, _ := res.RowsAffected()
	adminUserID := toIntFromContext(c, "userId")
	utils.LogActivity(
		h.db,
		int64(adminUserID),
		"delete_class",
		fmt.Sprintf("Admin menghapus kelas id=%s (rows_affected=%d)", classID, affected),
		c.ClientIP(),
		c.GetHeader("User-Agent"),
	)
	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Kelas berhasil dihapus"})
}

func (h *AdminHandler) GetMajors(c *gin.Context) {
	rows, err := h.db.Query(`SELECT id, name, code FROM majors ORDER BY name`)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	type Major struct {
		ID         int    `json:"id"`
		MajorName  string `json:"major_name"`
		MajorShort string `json:"major_short"`
	}
	var items []Major
	for rows.Next() {
		var item Major
		rows.Scan(&item.ID, &item.MajorName, &item.MajorShort)
		items = append(items, item)
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": items})
}

func (h *AdminHandler) CreateMajor(c *gin.Context) {
	var body struct {
		MajorName  string `json:"major_name" binding:"required"`
		MajorShort string `json:"major_short"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	_, err := h.db.Exec(`INSERT INTO majors (name, code) VALUES (?, ?)`, body.MajorName, body.MajorShort)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Raw(c, http.StatusCreated, gin.H{"success": true, "message": "Jurusan berhasil dibuat"})
}

func (h *AdminHandler) UpdateMajor(c *gin.Context) {
	var body struct {
		MajorName  string `json:"major_name"`
		MajorShort string `json:"major_short"`
	}
	c.ShouldBindJSON(&body)
	_, err := h.db.Exec(`UPDATE majors SET name = ?, code = ? WHERE id = ?`, body.MajorName, body.MajorShort, c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Jurusan berhasil diperbarui"})
}

func (h *AdminHandler) DeleteMajor(c *gin.Context) {
	majorID := c.Param("id")
	res, err := h.db.Exec(`DELETE FROM majors WHERE id = ?`, majorID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	affected, _ := res.RowsAffected()
	adminUserID := toIntFromContext(c, "userId")
	utils.LogActivity(
		h.db,
		int64(adminUserID),
		"delete_major",
		fmt.Sprintf("Admin menghapus jurusan id=%s (rows_affected=%d)", majorID, affected),
		c.ClientIP(),
		c.GetHeader("User-Agent"),
	)
	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Jurusan berhasil dihapus"})
}

func (h *AdminHandler) GetSubjects(c *gin.Context) {
	rows, err := h.db.Query(`SELECT id, name, code FROM subjects ORDER BY name`)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	type Subject struct {
		ID          int    `json:"id"`
		SubjectName string `json:"subject_name"`
		SubjectCode string `json:"subject_code"`
	}
	var items []Subject
	for rows.Next() {
		var item Subject
		rows.Scan(&item.ID, &item.SubjectName, &item.SubjectCode)
		items = append(items, item)
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": items})
}

func (h *AdminHandler) CreateSubject(c *gin.Context) {
	var body struct {
		SubjectName string `json:"subject_name" binding:"required"`
		SubjectCode string `json:"subject_code"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	_, err := h.db.Exec(`INSERT INTO subjects (name, code) VALUES (?, ?)`, body.SubjectName, body.SubjectCode)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Raw(c, http.StatusCreated, gin.H{"success": true, "message": "Mata pelajaran berhasil dibuat"})
}

func (h *AdminHandler) UpdateSubject(c *gin.Context) {
	var body struct {
		SubjectName string `json:"subject_name"`
		SubjectCode string `json:"subject_code"`
	}
	c.ShouldBindJSON(&body)
	_, err := h.db.Exec(`UPDATE subjects SET name = ?, code = ? WHERE id = ?`, body.SubjectName, body.SubjectCode, c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Mata pelajaran berhasil diperbarui"})
}

func (h *AdminHandler) DeleteSubject(c *gin.Context) {
	subjectID := c.Param("id")
	res, err := h.db.Exec(`DELETE FROM subjects WHERE id = ?`, subjectID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	affected, _ := res.RowsAffected()
	adminUserID := toIntFromContext(c, "userId")
	utils.LogActivity(
		h.db,
		int64(adminUserID),
		"delete_subject",
		fmt.Sprintf("Admin menghapus mata pelajaran id=%s (rows_affected=%d)", subjectID, affected),
		c.ClientIP(),
		c.GetHeader("User-Agent"),
	)
	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Mata pelajaran berhasil dihapus"})
}

func (h *AdminHandler) GetEnrollments(c *gin.Context) {
	classID := c.Query("class_id")
	query := `SELECT sce.id, sce.student_id, sce.class_id, sce.academic_year_id, COALESCE(sp.full_name,'') as student_name, COALESCE(sp.student_code,'') as student_code, sce.promotion_note
		FROM student_class_enrollments sce
		JOIN student_profiles sp ON sp.id = sce.student_id
		WHERE sce.is_active = 1`
	args := []any{}
	if classID != "" {
		query += " AND sce.class_id = ?"
		args = append(args, classID)
	}
	query += " ORDER BY sp.full_name LIMIT 200"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	type Enrollment struct {
		ID             int     `json:"id"`
		StudentID      int     `json:"student_id"`
		ClassID        int     `json:"class_id"`
		AcademicYearID int     `json:"academic_year_id"`
		StudentName    string  `json:"student_name"`
		StudentCode    string  `json:"student_code"`
		Notes          *string `json:"notes"`
	}
	var items []Enrollment
	for rows.Next() {
		var item Enrollment
		rows.Scan(&item.ID, &item.StudentID, &item.ClassID, &item.AcademicYearID, &item.StudentName, &item.StudentCode, &item.Notes)
		items = append(items, item)
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": items})
}

func (h *AdminHandler) CreateEnrollment(c *gin.Context) {
	var body struct {
		StudentID      int     `json:"student_id" binding:"required"`
		ClassID        int     `json:"class_id" binding:"required"`
		AcademicYearID int     `json:"academic_year_id"`
		Notes          *string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.AcademicYearID == 0 {
		h.db.QueryRow(`SELECT id FROM academic_years WHERE is_active = 1 LIMIT 1`).Scan(&body.AcademicYearID)
	}

	// Normalize notes: NULL/empty-string both become nil so the column stays NULL in DB.
	var notes *string
	if body.Notes != nil {
		trimmed := strings.TrimSpace(*body.Notes)
		if trimmed != "" {
			notes = &trimmed
		}
	}

	// Default to the existing promotion_note column. promotion_status stays 'active'
	// so the student is enrolled in the destination class for the target academic year.
	_, err := h.db.Exec(
		`INSERT INTO student_class_enrollments (student_id, class_id, academic_year_id, is_active, promotion_note) VALUES (?, ?, ?, 1, ?)`,
		body.StudentID, body.ClassID, body.AcademicYearID, notes,
	)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	adminUserID := toIntFromContext(c, "userId")
	noteSummary := ""
	if notes != nil {
		noteSummary = *notes
	}
	utils.LogActivity(
		h.db,
		int64(adminUserID),
		"create_enrollment",
		fmt.Sprintf("Admin mendaftarkan student_id=%d ke class_id=%d untuk academic_year_id=%d. Catatan: %q", body.StudentID, body.ClassID, body.AcademicYearID, noteSummary),
		c.ClientIP(),
		c.GetHeader("User-Agent"),
	)
	response.Raw(c, http.StatusCreated, gin.H{"success": true, "message": "Enrollment berhasil dibuat"})
}

func (h *AdminHandler) DeleteEnrollment(c *gin.Context) {
	enrollmentID := c.Param("id")
	res, err := h.db.Exec(`DELETE FROM student_class_enrollments WHERE id = ?`, enrollmentID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	affected, _ := res.RowsAffected()
	adminUserID := toIntFromContext(c, "userId")
	utils.LogActivity(
		h.db,
		int64(adminUserID),
		"delete_enrollment",
		fmt.Sprintf("Admin menghapus enrollment id=%s (rows_affected=%d)", enrollmentID, affected),
		c.ClientIP(),
		c.GetHeader("User-Agent"),
	)
	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Enrollment berhasil dihapus"})
}

func (h *AdminHandler) GetSchoolConfig(c *gin.Context) {
	rows, err := h.db.Query(`SELECT config_key, config_value FROM school_config`)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	config := map[string]string{}
	for rows.Next() {
		var k, v string
		rows.Scan(&k, &v)
		config[k] = v
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": config})
}

func (h *AdminHandler) GetPrincipalConfig(c *gin.Context) {
	var fullName, signatureURL string
	err := h.db.QueryRow(`
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
		// Return empty data rather than error so the print still works
		response.Raw(c, http.StatusOK, gin.H{
			"success": true,
			"data":    gin.H{"full_name": "", "signature_url": ""},
		})
		return
	}
	response.Raw(c, http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"full_name": fullName, "signature_url": signatureURL},
	})
}

func (h *AdminHandler) UpdateSchoolConfig(c *gin.Context) {
	var body map[string]string
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	for k, v := range body {
		h.db.Exec(`INSERT INTO school_config (config_key, config_value) VALUES (?, ?) ON DUPLICATE KEY UPDATE config_value = ?`, k, v, v)
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "message": "Konfigurasi berhasil diperbarui"})
}

func (h *AdminHandler) UploadConfigImage(c *gin.Context) {
	configKey := c.PostForm("config_key")
	if configKey == "" {
		response.Error(c, http.StatusBadRequest, "config_key diperlukan")
		return
	}

	// Validate config key
	allowedKeys := map[string]bool{
		"illustration_login_orange": true,
		"illustration_login_blue":   true,
		"illustration_register":     true,
		"bg_landing":                true,
		"app_logo":                  true,
		"school_logo":               true,
	}
	if !allowedKeys[configKey] {
		response.Error(c, http.StatusBadRequest, "config_key tidak valid")
		return
	}

	// Sanitize configKey: reject path separators and ".." sequences for defense-in-depth
	if strings.Contains(configKey, "/") || strings.Contains(configKey, "\\") || strings.Contains(configKey, "..") {
		response.Error(c, http.StatusBadRequest, "config_key tidak valid")
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "File diperlukan")
		return
	}

	// Limit 5MB
	if file.Size > 5*1024*1024 {
		response.Error(c, http.StatusBadRequest, "Ukuran file maksimal 5MB")
		return
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".svg" {
		response.Error(c, http.StatusBadRequest, "Format file tidak didukung (hanya PNG, JPG, JPEG, SVG)")
		return
	}

	contentType := file.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		response.Error(c, http.StatusBadRequest, "File harus berupa gambar")
		return
	}

	uploadDir := "public/uploads/config"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal membuat direktori upload")
		return
	}

	// Verify resolved paths stay within the upload directory
	absUploadDir, err := filepath.Abs(uploadDir)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal memproses path")
		return
	}

	// Clean up other extensions to prevent old file residue
	extensions := []string{".png", ".jpg", ".jpeg", ".svg"}
	for _, e := range extensions {
		if e != ext {
			candidatePath := filepath.Join(uploadDir, configKey+e)
			absCandidate, err := filepath.Abs(candidatePath)
			if err != nil || !strings.HasPrefix(absCandidate, absUploadDir) {
				continue
			}
			_ = os.Remove(candidatePath)
		}
	}

	filename := configKey + ext
	dst := filepath.Join(uploadDir, filename)

	// Verify the final destination path is within the upload directory
	absDst, err := filepath.Abs(dst)
	if err != nil || !strings.HasPrefix(absDst, absUploadDir) {
		response.Error(c, http.StatusBadRequest, "Path file tidak valid")
		return
	}

	if err := c.SaveUploadedFile(file, dst); err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal menyimpan file")
		return
	}

	filePath := "/uploads/config/" + filename
	_, dbErr := h.db.Exec(`INSERT INTO school_config (config_key, config_value) VALUES (?, ?) ON DUPLICATE KEY UPDATE config_value = ?`, configKey, filePath, filePath)
	if dbErr != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal menyimpan konfigurasi ke database: "+dbErr.Error())
		return
	}

	response.Raw(c, http.StatusOK, gin.H{
		"success": true,
		"message": "File berhasil diunggah",
		"data": gin.H{
			"config_key": configKey,
			"file_path":  filePath,
		},
	})
}

func (h *AdminHandler) GetAuditLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit := 50
	offset := (page - 1) * limit

	// Optional filters: activity_type, search (matches description or user_id).
	activityType := strings.TrimSpace(c.Query("activity_type"))
	search := strings.TrimSpace(c.Query("search"))

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

	// Total count.
	countQuery := "SELECT COUNT(*) FROM activity_logs" + whereSQL
	if err := h.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Page rows.
	listArgs := append(append([]any{}, args...), limit, offset)
	rows, err := h.db.Query(`
		SELECT id, user_id, activity_type AS action, description AS details, ip_address, user_agent, created_at
		FROM activity_logs`+whereSQL+`
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, listArgs...)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	type Log struct {
		ID        int     `json:"id"`
		UserID    *int    `json:"user_id"`
		Action    string  `json:"action"`
		Details   *string `json:"details"`
		IPAddress *string `json:"ip_address"`
		UserAgent *string `json:"user_agent"`
		CreatedAt string  `json:"created_at"`
	}
	logs := []Log{}
	for rows.Next() {
		var l Log
		if err := rows.Scan(&l.ID, &l.UserID, &l.Action, &l.Details, &l.IPAddress, &l.UserAgent, &l.CreatedAt); err != nil {
			response.Error(c, http.StatusInternalServerError, "Gagal memindai log: "+err.Error())
			return
		}
		logs = append(logs, l)
	}
	totalPages := (total + limit - 1) / limit
	response.Raw(c, http.StatusOK, gin.H{
		"success": true,
		"data":    logs,
		"meta": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}
