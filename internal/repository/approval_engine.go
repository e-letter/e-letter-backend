package repository

import (
	"database/sql"
	"errors"
	"fmt"
)

// ValidateApprovalStep verifies that prior steps are completed and the caller is authorized.
// Returns (callerTeacherID, callerPrincipalID, approverRole, isDelegated, delegatedFromID, error)
func ValidateApprovalStep(tx *sql.Tx, requestID int, stepNo int, approverUserID int) (int64, int64, string, bool, int64, error) {
	// 1. Fetch caller's profile IDs
	var callerTeacherID sql.NullInt64
	_ = tx.QueryRow(`SELECT id FROM teacher_profiles WHERE user_id = ? AND active = 1 AND deleted_at IS NULL`, approverUserID).Scan(&callerTeacherID)

	var callerPrincipalID sql.NullInt64
	_ = tx.QueryRow(`SELECT id FROM principal_profiles WHERE user_id = ? AND active = 1 AND deleted_at IS NULL`, approverUserID).Scan(&callerPrincipalID)

	// 2. Fetch all active steps for this request
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

	var steps []stepInfo
	var targetStep *stepInfo

	for rows.Next() {
		var s stepInfo
		if err := rows.Scan(&s.stepNo, &s.approverRole, &s.status, &s.approverTeacherID, &s.approverPrincipalID); err != nil {
			return 0, 0, "", false, 0, err
		}
		steps = append(steps, s)
		if s.stepNo == stepNo {
			targetStep = &s
		}
	}

	if targetStep == nil {
		return 0, 0, "", false, 0, errors.New("approval step not found for this request")
	}

	// 3. Verify sequential order: all steps with stepNo < targetStep.stepNo must be approved or skipped
	for _, s := range steps {
		if s.stepNo < stepNo {
			if s.status == "pending" {
				return 0, 0, "", false, 0, errors.New("Persetujuan harus dilakukan secara berurutan. Langkah sebelumnya masih tertunda.")
			}
		}
	}

	// 4. Verify target step is not already processed
	if targetStep.status != "pending" {
		return 0, 0, "", false, 0, errors.New("Langkah persetujuan ini sudah diproses.")
	}

	// 5. Check role authority
	isDelegated := false
	var delegatedFromID int64 = 0

	switch targetStep.approverRole {
	case "tatib":
		// Any teacher with the active 'tatib' role can approve
		if !callerTeacherID.Valid {
			return 0, 0, "", false, 0, errors.New("Anda harus memiliki profil guru yang aktif untuk menyetujui langkah ini.")
		}
		var hasTatibRole bool
		err = tx.QueryRow(`
			SELECT EXISTS(SELECT 1 FROM teacher_roles WHERE teacher_id = ? AND role_name = 'tatib' AND status = 'active')
		`, callerTeacherID.Int64).Scan(&hasTatibRole)
		if err != nil || !hasTatibRole {
			return 0, 0, "", false, 0, errors.New("Anda tidak memiliki peran aktif 'tatib' untuk menyetujui langkah ini.")
		}

	case "wali_kelas", "guru_mapel", "kapro":
		if !callerTeacherID.Valid {
			return 0, 0, "", false, 0, errors.New("Anda harus memiliki profil guru yang aktif untuk menyetujui langkah ini.")
		}
		assignedTeacherID := targetStep.approverTeacherID.Int64
		if !targetStep.approverTeacherID.Valid {
			return 0, 0, "", false, 0, errors.New("Tidak ada guru yang ditugaskan untuk langkah ini.")
		}

		if assignedTeacherID == callerTeacherID.Int64 {
			// Direct assigned teacher
			break
		}

		// Check active delegation
		var activeDelegationExists bool
		err = tx.QueryRow(`
			SELECT EXISTS(
				SELECT 1 FROM request_approval_delegates
				WHERE original_teacher_id = ? AND delegate_teacher_id = ? AND delegate_role = ?
				  AND is_active = 1 AND valid_from <= CURRENT_DATE() AND valid_until >= CURRENT_DATE()
			)
		`, assignedTeacherID, callerTeacherID.Int64, targetStep.approverRole).Scan(&activeDelegationExists)
		if err != nil || !activeDelegationExists {
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
