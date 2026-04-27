package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Refliqx/backend-eletter/internal/domain"
)

type PermissionRepository interface {
	domain.PermissionRepository
}

type permissionRepository struct {
	db *sql.DB
}

func NewPermissionRepository(db *sql.DB) PermissionRepository {
	return &permissionRepository{db: db}
}

func (r *permissionRepository) ListAll() ([]domain.PermissionRequest, error) {
	rows, err := r.db.Query(`SELECT request_id, type_id, no_surat, created_by, title, description, event_location, start_time, end_time, signature, status, created_at, updated_at FROM permission_requests ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPermissionRequests(rows)
}

func (r *permissionRepository) ListByUser(userID int) ([]domain.PermissionRequest, error) {
	rows, err := r.db.Query(`SELECT request_id, type_id, no_surat, created_by, title, description, event_location, start_time, end_time, signature, status, created_at, updated_at FROM permission_requests WHERE created_by = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPermissionRequests(rows)
}

func (r *permissionRepository) ListClasses() ([]domain.PermissionClass, error) {
	rows, err := r.db.Query(`SELECT class_id, class_name FROM classes ORDER BY class_name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.PermissionClass, 0)
	for rows.Next() {
		var c domain.PermissionClass
		if err := rows.Scan(&c.ClassID, &c.ClassName); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *permissionRepository) ListMajors() ([]domain.PermissionMajor, error) {
	rows, err := r.db.Query(`SELECT major_id, major_name, major_short FROM majors ORDER BY major_name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.PermissionMajor, 0)
	for rows.Next() {
		var m domain.PermissionMajor
		if err := rows.Scan(&m.MajorID, &m.MajorName, &m.MajorShort); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *permissionRepository) GetUserByNISN(nisn string) (*domain.User, error) {
	row := r.db.QueryRow(`
		SELECT u.id, COALESCE(u.login_code,''), COALESCE(u.role_id,0), u.email, u.password_hash,
		       up.full_name, up.nisn, up.nip, up.gender, up.phone_number, up.class_id,
		       COALESCE(up.can_request_dispensasi, false), COALESCE(up.profile_completed, false), u.is_active,
		       COALESCE(CAST(u.created_at AS TEXT),''), COALESCE(CAST(u.updated_at AS TEXT),'')
		FROM users u LEFT JOIN user_profiles up ON up.user_id = u.id
		WHERE up.nisn = $1 LIMIT 1`, nisn)
	return scanUser(row)
}

func (r *permissionRepository) GetUserByID(userID int) (*domain.User, error) {
	row := r.db.QueryRow(`
		SELECT u.id, COALESCE(u.login_code,''), COALESCE(u.role_id,0), u.email, u.password_hash,
		       up.full_name, up.nisn, up.nip, up.gender, up.phone_number, up.class_id,
		       COALESCE(up.can_request_dispensasi, false), COALESCE(up.profile_completed, false), u.is_active,
		       COALESCE(CAST(u.created_at AS TEXT),''), COALESCE(CAST(u.updated_at AS TEXT),'')
		FROM users u LEFT JOIN user_profiles up ON up.user_id = u.id
		WHERE u.id = $1 LIMIT 1`, userID)
	return scanUser(row)
}

func (r *permissionRepository) Create(req domain.CreatePermissionRequest) (int, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var id int
	if err := tx.QueryRow(
		`INSERT INTO permission_requests (type_id, created_by, title, description, event_location, start_time, end_time, status, request_date, is_active)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,'PENDING',NOW(),true) RETURNING request_id`,
		req.TypeID, req.IDSiswa, req.Title, req.Description, req.EventLocation, req.StartDate, req.EndDate,
	).Scan(&id); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func (r *permissionRepository) Update(req domain.UpdatePermissionRequest) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	updates := []string{}
	args := []any{}
	idx := 1
	if req.Title != nil {
		updates = append(updates, fmt.Sprintf("title = $%d", idx))
		args = append(args, *req.Title)
		idx++
	}
	if req.Description != nil {
		updates = append(updates, fmt.Sprintf("description = $%d", idx))
		args = append(args, *req.Description)
		idx++
	}
	if req.EventLocation != nil {
		updates = append(updates, fmt.Sprintf("event_location = $%d", idx))
		args = append(args, *req.EventLocation)
		idx++
	}
	if req.StartTime != nil {
		updates = append(updates, fmt.Sprintf("start_time = $%d", idx))
		args = append(args, *req.StartTime)
		idx++
	}
	if req.EndTime != nil {
		updates = append(updates, fmt.Sprintf("end_time = $%d", idx))
		args = append(args, *req.EndTime)
		idx++
	}
	if req.Status != nil {
		updates = append(updates, fmt.Sprintf("status = $%d", idx))
		args = append(args, *req.Status)
		idx++
	}
	updates = append(updates, "updated_at = NOW()")
	if len(updates) == 1 {
		return nil
	}
	args = append(args, req.RequestID)
	query := fmt.Sprintf("UPDATE permission_requests SET %s WHERE request_id = $%d", strings.Join(updates, ", "), idx)
	if _, err := tx.Exec(query, args...); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *permissionRepository) Delete(requestID int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM permission_requests WHERE request_id = $1`, requestID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *permissionRepository) Approve(req domain.ApprovalRequest, approverID int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var typeID int
	if err := tx.QueryRow(`SELECT type_id FROM permission_requests WHERE request_id = $1 FOR UPDATE`, req.RequestID).Scan(&typeID); err != nil {
		return err
	}

	var stageExists bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM approval_stages WHERE stage_id = $1 AND type_id = $2)`, req.StageID, typeID).Scan(&stageExists); err != nil {
		return err
	}
	if !stageExists {
		return errors.New("Approval stage not found or does not belong to request type")
	}

	if _, err := tx.Exec(
		`INSERT INTO permission_approval_logs (request_id, stage_id, approver_id, status, notes, signature_url, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,NOW())`,
		req.RequestID, req.StageID, approverID, req.Status, req.Notes, req.SignatureURL,
	); err != nil {
		return err
	}

	targetStatus := "PARTIALLY_APPROVED"
	if req.Status == "REJECTED" {
		targetStatus = "REJECTED"
	} else if req.Status == "APPROVED" {
		var requiredCount, approvedCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM approval_stages WHERE type_id = $1 AND required = true`, typeID).Scan(&requiredCount); err != nil {
			return err
		}
		if err := tx.QueryRow(`
			SELECT COUNT(DISTINCT pal.stage_id)
			FROM permission_approval_logs pal
			JOIN approval_stages s ON s.stage_id = pal.stage_id
			WHERE pal.request_id = $1 AND pal.status = 'APPROVED' AND s.required = true`,
			req.RequestID,
		).Scan(&approvedCount); err != nil {
			return err
		}
		if approvedCount >= requiredCount {
			targetStatus = "APPROVED"
		}
	}

	if _, err := tx.Exec(`UPDATE permission_requests SET status = $1, updated_at = NOW() WHERE request_id = $2`, targetStatus, req.RequestID); err != nil {
		return err
	}

	if _, err := tx.Exec(
		`INSERT INTO audit_logs (entity_type, entity_id, action, performed_by, payload, created_at)
		 VALUES ('permission_requests', $1, $2, $3, $4, NOW())`,
		req.RequestID, strings.ToLower(targetStatus), approverID, fmt.Sprintf(`{"stage_id":%d}`, req.StageID),
	); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *permissionRepository) ListRegistrationTokens() ([]domain.TokenRecord, error) {
	rows, err := r.db.Query(`SELECT token_id, COALESCE(user_id,0), token_hash, token_type, COALESCE(is_revoked,false), expires_at, usage_limit, used_count FROM tokens WHERE token_type='registration' ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.TokenRecord, 0)
	for rows.Next() {
		var rec domain.TokenRecord
		var usageLimit sql.NullInt64
		if err := rows.Scan(&rec.TokenID, &rec.UserID, &rec.TokenHash, &rec.TokenType, &rec.IsRevoked, &rec.ExpiresAt, &usageLimit, &rec.UsedCount); err != nil {
			return nil, err
		}
		if usageLimit.Valid {
			v := int(usageLimit.Int64)
			rec.UsageLimit = &v
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (r *permissionRepository) CreateOrUpdateRegistrationToken(token string, roleID int, usageLimit *int, expiresAt *time.Time) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO tokens (token_hash, token_type, usage_limit, used_count, expires_at, is_revoked)
		 VALUES ($1, 'registration', $2, 0, $3, false)
		 ON CONFLICT (token_hash) DO UPDATE SET usage_limit = EXCLUDED.usage_limit, expires_at = EXCLUDED.expires_at`,
		token, usageLimit, expiresAt,
	); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	_ = roleID
	return nil
}

func (r *permissionRepository) GetRegistrationTokenByValue(token string) (*domain.TokenRecord, error) {
	row := r.db.QueryRow(`SELECT token_id, COALESCE(user_id,0), token_hash, token_type, COALESCE(is_revoked,false), expires_at, usage_limit, used_count FROM tokens WHERE token_hash = $1 AND token_type = 'registration' LIMIT 1`, token)
	var rec domain.TokenRecord
	var usageLimit sql.NullInt64
	if err := row.Scan(&rec.TokenID, &rec.UserID, &rec.TokenHash, &rec.TokenType, &rec.IsRevoked, &rec.ExpiresAt, &usageLimit, &rec.UsedCount); err != nil {
		return nil, err
	}
	if usageLimit.Valid {
		v := int(usageLimit.Int64)
		rec.UsageLimit = &v
	}
	return &rec, nil
}

func scanPermissionRequests(rows *sql.Rows) ([]domain.PermissionRequest, error) {
	out := make([]domain.PermissionRequest, 0)
	for rows.Next() {
		var req domain.PermissionRequest
		if err := rows.Scan(
			&req.RequestID,
			&req.TypeID,
			&req.NoSurat,
			&req.CreatedBy,
			&req.Title,
			&req.Description,
			&req.EventLocation,
			&req.StartTime,
			&req.EndTime,
			&req.Signature,
			&req.Status,
			&req.CreatedAt,
			&req.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	return out, rows.Err()
}
