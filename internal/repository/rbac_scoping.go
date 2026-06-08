package repository

import (
	"database/sql"
	"fmt"
	"strings"
)

func BuildRBACScopeFilter(db *sql.DB, userID int) (string, error) {
	var principalID int64
	err := db.QueryRow(`
		SELECT id FROM principal_profiles
		WHERE user_id = ? AND active = 1 AND deleted_at IS NULL
		LIMIT 1
	`, userID).Scan(&principalID)
	if err == nil {
		return "1=1", nil
	}
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}

	rows, err := db.Query(`
		SELECT tp.id AS teacher_id, tr.role_name
		FROM teacher_profiles tp
		JOIN teacher_roles tr ON tr.teacher_id = tp.id AND tr.status = 'active'
		WHERE tp.user_id = ? AND tp.active = 1 AND tp.deleted_at IS NULL
	`, userID)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	type teacherRole struct {
		teacherID int64
		roleName  string
	}
	var roles []teacherRole
	for rows.Next() {
		var r teacherRole
		if err := rows.Scan(&r.teacherID, &r.roleName); err != nil {
			return "", err
		}
		roles = append(roles, r)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	if len(roles) == 0 {
		return "1=0", nil
	}

	for _, r := range roles {
		if r.roleName == "tatib" {
			return "1=1", nil
		}
	}

	var academicYearID int64
	err = db.QueryRow(`SELECT id FROM academic_years WHERE is_active = 1 LIMIT 1`).Scan(&academicYearID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "1=0", nil
		}
		return "", err
	}

	var conditions []string
	for _, r := range roles {
		tid := r.teacherID
		yid := academicYearID
		switch r.roleName {
		case "wali_kelas":
			conditions = append(conditions, fmt.Sprintf(
				"sce.class_id IN (SELECT class_id FROM class_homeroom_assignments WHERE teacher_id = %d AND academic_year_id = %d AND is_active = 1)",
				tid, yid))
		case "guru_mapel":
			conditions = append(conditions, fmt.Sprintf(
				"sce.class_id IN (SELECT class_id FROM schedules WHERE teacher_id = %d AND academic_year_id = %d AND is_active = 1)",
				tid, yid))
		case "kapro":
			conditions = append(conditions, fmt.Sprintf(
				"sce.class_id IN (SELECT cl.id FROM classes cl JOIN major_head_assignments mha ON cl.major_id = mha.major_id WHERE mha.teacher_id = %d AND mha.academic_year_id = %d AND mha.is_active = 1 AND cl.is_active = 1)",
				tid, yid))
		}
	}

	if len(conditions) == 0 {
		return "1=0", nil
	}

	return "(" + strings.Join(conditions, " OR ") + ")", nil
}
