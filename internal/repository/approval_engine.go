package repository

import (
	"database/sql"
	"errors"
	"fmt"
)

func ValidateApprovalStep(tx *sql.Tx, requestID int, stepNo int, approverUserID int) (int64, int64, string, bool, int64, error) {
	var callerTeacherID sql.NullInt64
	var callerPrincipalID sql.NullInt64
	err := tx.QueryRow(`
		SELECT tp.id, pp.id
		FROM users u
		LEFT JOIN teacher_profiles tp ON tp.user_id = u.id AND tp.active = 1 AND tp.deleted_at IS NULL
		LEFT JOIN principal_profiles pp ON pp.user_id = u.id AND pp.active = 1 AND pp.deleted_at IS NULL
		WHERE u.id = ? AND u.deleted_at IS NULL
	`, approverUserID).Scan(&callerTeacherID, &callerPrincipalID)
	if err != nil && err != sql.ErrNoRows {
		return 0, 0, "", false, 0, fmt.Errorf("resolve approver profiles: %w", err)
	}

	var elapsedMinutes int
	var requestCode string
	err = tx.QueryRow(`
		SELECT TIMESTAMPDIFF(MINUTE, r.submitted_at, NOW()), rt.code
		FROM requests r
		JOIN request_types rt ON rt.id = r.request_type_id
		WHERE r.id = ?
	`, requestID).Scan(&elapsedMinutes, &requestCode)
	if err != nil {
		return 0, 0, "", false, 0, err
	}

	isIzinKeluarLate := requestCode == "izin_keluar" && elapsedMinutes > 30

	rows, err := tx.Query(`
		SELECT step_no, approver_role, status, approver_teacher_id, approver_principal_id
		FROM request_approvals
		WHERE request_id = ? AND deleted_at IS NULL
		ORDER BY step_no ASC
	`, requestID)
	if err != nil {
		return 0, 0, "", false, 0, err
	}
	defer rows.Close()

	type stepInfo struct {
		stepNo              int
		approverRole        string
		status              string
		approverTeacherID   sql.NullInt64
		approverPrincipalID sql.NullInt64
	}

	var allSteps []stepInfo
	var targetStep *stepInfo

	for rows.Next() {
		var s stepInfo
		if err := rows.Scan(&s.stepNo, &s.approverRole, &s.status, &s.approverTeacherID, &s.approverPrincipalID); err != nil {
			return 0, 0, "", false, 0, err
		}
		allSteps = append(allSteps, s)
		if s.stepNo == stepNo {
			cp := s
			targetStep = &cp
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, "", false, 0, err
	}

	if targetStep == nil {
		return 0, 0, "", false, 0, errors.New("approval step not found for this request")
	}

	callerIsKapro := false
	if isIzinKeluarLate && callerTeacherID.Valid {
		if err := tx.QueryRow(`
			SELECT EXISTS(
				SELECT 1 FROM teacher_roles
				WHERE teacher_id = ? AND role_name = 'kapro' AND status = 'active'
			)
		`, callerTeacherID.Int64).Scan(&callerIsKapro); err != nil {
			return 0, 0, "", false, 0, fmt.Errorf("check kapro role: %w", err)
		}
	}

	bypassableRoles := map[string]bool{"guru_mapel": true}
	isKaproBypassingThisStep := isIzinKeluarLate && callerIsKapro && bypassableRoles[targetStep.approverRole]

	if !isKaproBypassingThisStep {
		for _, s := range allSteps {
			if s.stepNo < stepNo && s.status == "pending" {
				return 0, 0, "", false, 0, errors.New(
					"Persetujuan harus dilakukan secara berurutan. Langkah sebelumnya masih tertunda.",
				)
			}
		}
	}

	if targetStep.status != "pending" {
		return 0, 0, "", false, 0, errors.New("Langkah persetujuan ini sudah diproses.")
	}

	isDelegated := false
	var delegatedFromID int64

	if isKaproBypassingThisStep {
		return callerTeacherID.Int64, 0, targetStep.approverRole, false, 0, nil
	}

	switch targetStep.approverRole {
	case "tatib":
		if !callerTeacherID.Valid {
			return 0, 0, "", false, 0, errors.New("Anda harus memiliki profil guru yang aktif untuk menyetujui langkah ini.")
		}
		var hasTatibRole bool
		if err = tx.QueryRow(`
			SELECT EXISTS(
				SELECT 1 FROM teacher_roles
				WHERE teacher_id = ? AND role_name = 'tatib' AND status = 'active'
			)
		`, callerTeacherID.Int64).Scan(&hasTatibRole); err != nil {
			return 0, 0, "", false, 0, fmt.Errorf("check tatib role: %w", err)
		}
		if !hasTatibRole {
			return 0, 0, "", false, 0, errors.New("Anda tidak memiliki peran aktif 'tatib' untuk menyetujui langkah ini.")
		}

	case "wali_kelas", "guru_mapel":
		if !callerTeacherID.Valid {
			return 0, 0, "", false, 0, errors.New("Anda harus memiliki profil guru yang aktif untuk menyetujui langkah ini.")
		}
		if !targetStep.approverTeacherID.Valid {
			return 0, 0, "", false, 0, errors.New("Tidak ada guru yang ditugaskan untuk langkah ini.")
		}
		assignedTeacherID := targetStep.approverTeacherID.Int64

		if assignedTeacherID == callerTeacherID.Int64 {
			break
		}

		var activeDelegationExists bool
		if err = tx.QueryRow(`
			SELECT EXISTS(
				SELECT 1 FROM request_approval_delegates
				WHERE original_teacher_id = ? AND delegate_teacher_id = ? AND delegate_role = ?
				  AND is_active = 1 AND valid_from <= CURRENT_DATE() AND valid_until >= CURRENT_DATE()
			)
		`, assignedTeacherID, callerTeacherID.Int64, targetStep.approverRole).Scan(&activeDelegationExists); err != nil {
			return 0, 0, "", false, 0, fmt.Errorf("check delegation: %w", err)
		}
		if !activeDelegationExists {
			return 0, 0, "", false, 0, errors.New("Anda tidak berwenang menyetujui langkah ini (bukan guru yang ditugaskan atau delegasi aktif).")
		}

		isDelegated = true
		delegatedFromID = assignedTeacherID

	case "kepala_sekolah":
		if !callerPrincipalID.Valid {
			return 0, 0, "", false, 0, errors.New("Anda harus memiliki profil kepala sekolah yang aktif untuk menyetujui langkah ini.")
		}
		if targetStep.approverPrincipalID.Valid && targetStep.approverPrincipalID.Int64 != callerPrincipalID.Int64 {
			return 0, 0, "", false, 0, errors.New("Anda bukan kepala sekolah yang ditugaskan untuk langkah ini.")
		}

	default:
		return 0, 0, "", false, 0, fmt.Errorf("peran approver tidak valid: %s", targetStep.approverRole)
	}

	return callerTeacherID.Int64, callerPrincipalID.Int64, targetStep.approverRole, isDelegated, delegatedFromID, nil
}
