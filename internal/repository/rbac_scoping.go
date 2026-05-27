package repository

import (
	"database/sql"
	"fmt"
	"strings"
)

// BuildRBACScopeFilter generates a deny-by-default SQL condition for student scoping based on active teacher roles.
func BuildRBACScopeFilter(db *sql.DB, userID int) (string, error) {
	var teacherID int64
	err := db.QueryRow(`SELECT id FROM teacher_profiles WHERE user_id = ? AND active = 1 AND deleted_at IS NULL`, userID).Scan(&teacherID)
	if err != nil {
		if err == sql.ErrNoRows {
			// No active teacher profile: deny all.
			return "1=0", nil
		}
		return "", err
	}

	var academicYearID int64
	err = db.QueryRow(`SELECT id FROM academic_years WHERE is_active = 1 LIMIT 1`).Scan(&academicYearID)
	if err != nil {
		if err == sql.ErrNoRows {
			// No active academic year: deny all.
			return "1=0", nil
		}
		return "", err
	}

	rows, err := db.Query(`SELECT role_name FROM teacher_roles WHERE teacher_id = ? AND status = 'active'`, teacherID)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var conditions []string
	hasActiveRole := false

	for rows.Next() {
		var roleName string
		if err := rows.Scan(&roleName); err != nil {
			return "", err
		}
		hasActiveRole = true
		switch roleName {
		case "tatib":
			// Tatib has unrestricted access to all students/classes.
			return "1=1", nil
		case "wali_kelas":
			conditions = append(conditions, fmt.Sprintf("sce.class_id IN (SELECT class_id FROM class_homeroom_assignments WHERE teacher_id = %d AND academic_year_id = %d AND is_active = 1)", teacherID, academicYearID))
		case "guru_mapel":
			conditions = append(conditions, fmt.Sprintf("sce.class_id IN (SELECT class_id FROM schedules WHERE teacher_id = %d AND academic_year_id = %d AND is_active = 1)", teacherID, academicYearID))
		case "kapro":
			conditions = append(conditions, fmt.Sprintf("sce.class_id IN (SELECT cl.id FROM classes cl JOIN major_head_assignments mha ON cl.major_id = mha.major_id WHERE mha.teacher_id = %d AND mha.academic_year_id = %d AND mha.is_active = 1 AND cl.is_active = 1)", teacherID, academicYearID))
		}
	}

	if !hasActiveRole || len(conditions) == 0 {
		return "1=0", nil
	}

	return "(" + strings.Join(conditions, " OR ") + ")", nil
}
