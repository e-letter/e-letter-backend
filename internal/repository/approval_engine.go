package repository

import (
	"database/sql"
	"errors"
	"fmt"
)

// ValidateApprovalStep verifies that prior steps are completed and the caller is authorized.
// Returns (callerTeacherID, callerPrincipalID, approverRole, isDelegated, delegatedFromID, error)
//
// Approval flow for izin_keluar (RBAC.md §9):
//
//	Normal (≤30 min): guru_mapel (step 1) → wali_kelas (step 2) → tatib (step 3)
//	  – strictly sequential; each role must approve before the next step unlocks.
//
//	Bypass (>30 min): kapro is NOT a step — it is a bypass authority only.
//	  – If guru_mapel and/or wali_kelas have not approved within 30 minutes, kapro may
//	    approve their pending steps on their behalf (those steps are auto-skipped in Approve()).
//	  – Tatib is ALWAYS the mandatory final approver and is NEVER bypassable.
//
//	Multi-role: if the approver holds multiple overlapping roles, Approve() cascades
//	  auto-approvals across all steps they are authorised for.
func ValidateApprovalStep(tx *sql.Tx, requestID int, stepNo int, approverUserID int) (int64, int64, string, bool, int64, error) {
	// 1. Resolve caller's internal profile IDs
	var callerTeacherID sql.NullInt64
	_ = tx.QueryRow(
		`SELECT id FROM teacher_profiles WHERE user_id = ? AND active = 1 AND deleted_at IS NULL`,
		approverUserID,
	).Scan(&callerTeacherID)

	var callerPrincipalID sql.NullInt64
	_ = tx.QueryRow(
		`SELECT id FROM principal_profiles WHERE user_id = ? AND active = 1 AND deleted_at IS NULL`,
		approverUserID,
	).Scan(&callerPrincipalID)

	// 2. Check elapsed time since the request was submitted (use submitted_at for precision)
	var elapsedMinutes int
	var requestCode string
	err := tx.QueryRow(`
		SELECT TIMESTAMPDIFF(MINUTE, r.submitted_at, NOW()), rt.code
		FROM requests r
		JOIN request_types rt ON rt.id = r.request_type_id
		WHERE r.id = ?
	`, requestID).Scan(&elapsedMinutes, &requestCode)
	if err != nil {
		return 0, 0, "", false, 0, err
	}

	// isIzinKeluarLate: true when the 30-minute window has elapsed for an izin_keluar request
	isIzinKeluarLate := requestCode == "izin_keluar" && elapsedMinutes > 30

	// 3. Load all approval steps for sequential-order verification
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

	if targetStep == nil {
		return 0, 0, "", false, 0, errors.New("approval step not found for this request")
	}

	// 4. Determine whether the caller is kapro (required for the bypass eligibility check)
	callerIsKapro := false
	if isIzinKeluarLate && callerTeacherID.Valid {
		_ = tx.QueryRow(`
			SELECT EXISTS(
				SELECT 1 FROM teacher_roles
				WHERE teacher_id = ? AND role_name = 'kapro' AND status = 'active'
			)
		`, callerTeacherID.Int64).Scan(&callerIsKapro)
	}

	// bypassableRoles: roles that kapro may approve on behalf of after the 30-minute window.
	// tatib is intentionally excluded — it is always the mandatory final approver.
	bypassableRoles := map[string]bool{"guru_mapel": true, "wali_kelas": true}
	isKaproBypassingThisStep := isIzinKeluarLate && callerIsKapro && bypassableRoles[targetStep.approverRole]

	// 5. Sequential-order enforcement.
	//    Kapro may skip over still-pending guru_mapel/wali_kelas steps after 30 minutes.
	//    Tatib step is ALWAYS sequential regardless of elapsed time.
	if !isKaproBypassingThisStep {
		for _, s := range allSteps {
			if s.stepNo < stepNo && s.status == "pending" {
				return 0, 0, "", false, 0, errors.New(
					"Persetujuan harus dilakukan secara berurutan. Langkah sebelumnya masih tertunda.",
				)
			}
		}
	}

	// 6. Guard: step must still be pending
	if targetStep.status != "pending" {
		return 0, 0, "", false, 0, errors.New("Langkah persetujuan ini sudah diproses.")
	}

	// 7. Role-authority check
	isDelegated := false
	var delegatedFromID int64

	// Kapro bypass path: kapro approves on behalf of guru_mapel or wali_kelas.
	// The Approve() function will record kapro's teacher ID on the step and then
	// auto-skip any remaining bypassable pending steps.
	if isKaproBypassingThisStep {
		return callerTeacherID.Int64, 0, targetStep.approverRole, false, 0, nil
	}

	switch targetStep.approverRole {
	case "tatib":
		// Any teacher holding an active 'tatib' role may approve
		if !callerTeacherID.Valid {
			return 0, 0, "", false, 0, errors.New("Anda harus memiliki profil guru yang aktif untuk menyetujui langkah ini.")
		}
		var hasTatibRole bool
		if err = tx.QueryRow(`
			SELECT EXISTS(
				SELECT 1 FROM teacher_roles
				WHERE teacher_id = ? AND role_name = 'tatib' AND status = 'active'
			)
		`, callerTeacherID.Int64).Scan(&hasTatibRole); err != nil || !hasTatibRole {
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
			break // direct match
		}

		// Check for active delegation from the assigned teacher to the caller
		var activeDelegationExists bool
		if err = tx.QueryRow(`
			SELECT EXISTS(
				SELECT 1 FROM request_approval_delegates
				WHERE original_teacher_id = ? AND delegate_teacher_id = ? AND delegate_role = ?
				  AND is_active = 1 AND valid_from <= CURRENT_DATE() AND valid_until >= CURRENT_DATE()
			)
		`, assignedTeacherID, callerTeacherID.Int64, targetStep.approverRole).Scan(&activeDelegationExists); err != nil || !activeDelegationExists {
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
