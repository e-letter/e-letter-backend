package repository

import (
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMockTx(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *sql.Tx) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	return db, mock, tx
}

func stepRow(stepNo int, role, status string, teacherID interface{}) *sqlmock.Rows {
	return sqlmock.NewRows(
		[]string{"step_no", "approver_role", "status", "approver_teacher_id", "approver_principal_id"},
	).AddRow(stepNo, role, status, teacherID, nil)
}

func TestSequentialGuard_PriorStepPending(t *testing.T) {
	_, mock, tx := newMockTx(t)

	const (
		approverUserID = 5
		requestID      = 10
		stepNo         = 2
	)

	mock.ExpectQuery(`SELECT tp\.id, pp\.id`).
		WithArgs(approverUserID).
		WillReturnRows(sqlmock.NewRows([]string{"tp_id", "pp_id"}).AddRow(5, nil))

	mock.ExpectQuery(`SELECT TIMESTAMPDIFF`).
		WithArgs(requestID).
		WillReturnRows(sqlmock.NewRows([]string{"elapsed", "code"}).AddRow(5, "izin_keluar"))

	twoStepRows := sqlmock.NewRows(
		[]string{"step_no", "approver_role", "status", "approver_teacher_id", "approver_principal_id"},
	).
		AddRow(1, "guru_mapel", "pending", 3, nil).
		AddRow(2, "tatib", "pending", 5, nil)

	mock.ExpectQuery(`SELECT step_no, approver_role, status`).
		WithArgs(requestID).
		WillReturnRows(twoStepRows)

	_, _, _, _, _, err := ValidateApprovalStep(tx, requestID, stepNo, approverUserID)

	assert.ErrorContains(t, err, "berurutan",
		"expected sequential-guard error; got: %v", err)

	_ = tx.Rollback()
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSequentialGuard_StepNotFound(t *testing.T) {
	_, mock, tx := newMockTx(t)

	const (
		approverUserID = 5
		requestID      = 99
		stepNo         = 9
	)

	mock.ExpectQuery(`SELECT tp\.id, pp\.id`).
		WithArgs(approverUserID).
		WillReturnRows(sqlmock.NewRows([]string{"tp_id", "pp_id"}).AddRow(5, nil))

	mock.ExpectQuery(`SELECT TIMESTAMPDIFF`).
		WithArgs(requestID).
		WillReturnRows(sqlmock.NewRows([]string{"elapsed", "code"}).AddRow(2, "izin_masuk"))

	mock.ExpectQuery(`SELECT step_no, approver_role, status`).
		WithArgs(requestID).
		WillReturnRows(stepRow(1, "tatib", "pending", 5))

	_, _, _, _, _, err := ValidateApprovalStep(tx, requestID, stepNo, approverUserID)

	assert.ErrorContains(t, err, "not found",
		"expected step-not-found error; got: %v", err)

	_ = tx.Rollback()
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestKaproBypass_SkipsGuruMapelStep(t *testing.T) {
	_, mock, tx := newMockTx(t)

	const (
		approverUserID = 8
		requestID      = 20
		stepNo         = 1
	)

	mock.ExpectQuery(`SELECT tp\.id, pp\.id`).
		WithArgs(approverUserID).
		WillReturnRows(sqlmock.NewRows([]string{"tp_id", "pp_id"}).AddRow(8, nil))

	mock.ExpectQuery(`SELECT TIMESTAMPDIFF`).
		WithArgs(requestID).
		WillReturnRows(sqlmock.NewRows([]string{"elapsed", "code"}).AddRow(35, "izin_keluar"))

	mock.ExpectQuery(`SELECT step_no, approver_role, status`).
		WithArgs(requestID).
		WillReturnRows(sqlmock.NewRows(
			[]string{"step_no", "approver_role", "status", "approver_teacher_id", "approver_principal_id"},
		).
			AddRow(1, "guru_mapel", "pending", 3, nil).
			AddRow(2, "tatib", "pending", 5, nil))

	mock.ExpectQuery(`SELECT EXISTS.*teacher_roles`).
		WithArgs(int64(8)).
		WillReturnRows(sqlmock.NewRows([]string{"is_kapro"}).AddRow(1))

	mock.ExpectQuery(`SELECT status FROM request_approvals`).
		WithArgs(requestID, stepNo).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("pending"))

	mock.ExpectQuery(`SELECT EXISTS.*teacher_roles`).
		WithArgs(int64(8)).
		WillReturnRows(sqlmock.NewRows([]string{"has_role"}).AddRow(1))

	_, _, role, isDelegated, _, err := ValidateApprovalStep(tx, requestID, stepNo, approverUserID)

	if err != nil {
		assert.NotContains(t, err.Error(), "berurutan",
			"kapro bypass must NOT trigger the sequential guard; got: %v", err)
	} else {
		assert.Equal(t, "guru_mapel", role)
		assert.False(t, isDelegated, "kapro bypass returns isDelegated=false by design")
	}

	_ = tx.Rollback()
}

func TestTatibAlwaysMandatory(t *testing.T) {
	_, mock, tx := newMockTx(t)

	const (
		approverUserID = 8
		requestID      = 30
		stepNo         = 2
	)

	mock.ExpectQuery(`SELECT tp\.id, pp\.id`).
		WithArgs(approverUserID).
		WillReturnRows(sqlmock.NewRows([]string{"tp_id", "pp_id"}).AddRow(8, nil))

	mock.ExpectQuery(`SELECT TIMESTAMPDIFF`).
		WithArgs(requestID).
		WillReturnRows(sqlmock.NewRows([]string{"elapsed", "code"}).AddRow(45, "izin_keluar"))

	mock.ExpectQuery(`SELECT step_no, approver_role, status`).
		WithArgs(requestID).
		WillReturnRows(sqlmock.NewRows(
			[]string{"step_no", "approver_role", "status", "approver_teacher_id", "approver_principal_id"},
		).
			AddRow(1, "guru_mapel", "approved", 3, nil).
			AddRow(2, "tatib", "pending", 5, nil))

	mock.ExpectQuery(`SELECT EXISTS.*teacher_roles`).
		WithArgs(int64(8)).
		WillReturnRows(sqlmock.NewRows([]string{"is_kapro"}).AddRow(1))

	mock.ExpectQuery(`SELECT status FROM request_approvals`).
		WithArgs(requestID, stepNo).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("pending"))

	mock.ExpectQuery(`SELECT EXISTS.*teacher_roles`).
		WithArgs(int64(8)).
		WillReturnRows(sqlmock.NewRows([]string{"has_role"}).AddRow(0))

	_, _, _, _, _, err := ValidateApprovalStep(tx, requestID, stepNo, approverUserID)

	if err != nil {
		assert.NotContains(t, err.Error(), "berurutan",
			"tatib should NOT fail sequential guard when guru_mapel is approved")
	}

	_ = tx.Rollback()
}

var _ = time.Now
