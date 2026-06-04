package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/Refliqx/backend-eletter/internal/domain"
)

type NotificationRepository interface {
	GetByUser(ctx context.Context, userID int64) ([]domain.Notification, error)
	MarkAsRead(ctx context.Context, notificationID, userID int64) error
	Create(ctx context.Context, userID int64, notifType, title string, body *string, requestID, approvalID *int64) error
}

type notificationRepository struct {
	db *sql.DB
}

func NewNotificationRepository(db *sql.DB) NotificationRepository {
	return &notificationRepository{db: db}
}

func (r *notificationRepository) GetByUser(ctx context.Context, userID int64) ([]domain.Notification, error) {
	query := `
		SELECT id, user_id, request_id, approval_id, type, title, body, is_read, read_at, created_at
		FROM notifications
		WHERE user_id = ?
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notifications []domain.Notification
	for rows.Next() {
		var n domain.Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.RequestID, &n.ApprovalID, &n.Type, &n.Title, &n.Body, &n.IsRead, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, err
		}
		notifications = append(notifications, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Track request IDs already covered by real notifications
	existingReqIDs := make(map[int64]bool)
	for _, n := range notifications {
		if n.RequestID != nil {
			existingReqIDs[*n.RequestID] = true
		}
	}

	// Add virtual notifications from requests where user is requester
	virtualID := int64(-1)
	virtualNotifs, err := r.getRequestsAsNotifications(ctx, userID, existingReqIDs, &virtualID)
	if err != nil {
		// Non-fatal: return what we have
		return notifications, nil
	}
	notifications = append(notifications, virtualNotifs...)

	// Sort by created_at DESC
	sort.Slice(notifications, func(i, j int) bool {
		return notifications[i].CreatedAt.After(notifications[j].CreatedAt)
	})

	return notifications, nil
}

type virtualRow struct {
	RequestID     int64
	ApprovalID    *int64
	ReqTypeCode   string
	ReqTypeLabel  string
	Reason        *string
	ReqStatus     string
	CreatedAt     time.Time
	RequesterName string
}

func (r *notificationRepository) getRequestsAsNotifications(ctx context.Context, userID int64, existingReqIDs map[int64]bool, virtualID *int64) ([]domain.Notification, error) {
	var notifications []domain.Notification

	// Query 1: Requests where user is the requester
	rows1, err := r.db.QueryContext(ctx, `
		SELECT r.id, NULL, rt.code, rt.label, r.reason, r.status, COALESCE(r.submitted_at, r.created_at), ''
		FROM requests r
		JOIN request_types rt ON rt.id = r.request_type_id
		WHERE r.requester_user_id = ? AND r.deleted_at IS NULL AND r.status != 'draft'
	`, userID)
	if err == nil {
		defer rows1.Close()
		for rows1.Next() {
			var v virtualRow
			if err := rows1.Scan(&v.RequestID, &v.ApprovalID, &v.ReqTypeCode, &v.ReqTypeLabel, &v.Reason, &v.ReqStatus, &v.CreatedAt, &v.RequesterName); err != nil {
				continue
			}
			if existingReqIDs[v.RequestID] {
				continue
			}
			n := r.buildVirtualNotification(userID, v, false, *virtualID)
			notifications = append(notifications, n)
			*virtualID--
		}
		rows1.Close()
	}

	// Query 2: Requests where user is an approver (via teacher_profiles or principal_profiles)
	rows2, err := r.db.QueryContext(ctx, `
		SELECT r.id, ra.id, rt.code, rt.label, r.reason, ra.status, COALESCE(ra.acted_at, r.created_at), ''
		FROM request_approvals ra
		JOIN requests r ON r.id = ra.request_id AND r.deleted_at IS NULL AND r.status != 'draft'
		JOIN request_types rt ON rt.id = r.request_type_id
		LEFT JOIN teacher_profiles tp ON tp.id = ra.approver_teacher_id
		LEFT JOIN principal_profiles pp ON pp.id = ra.approver_principal_id
		WHERE tp.user_id = ? OR pp.user_id = ?
	`, userID, userID)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var (
				reqID, appID  int64
				code, label   string
				reason        *string
				status        string
				createdAt     time.Time
				requesterName string
			)
			if err := rows2.Scan(&reqID, &appID, &code, &label, &reason, &status, &createdAt, &requesterName); err != nil {
				continue
			}
			if existingReqIDs[reqID] {
				continue
			}
			v := virtualRow{
				RequestID:     reqID,
				ApprovalID:    &appID,
				ReqTypeCode:   code,
				ReqTypeLabel:  label,
				Reason:        reason,
				ReqStatus:     status,
				CreatedAt:     createdAt,
				RequesterName: requesterName,
			}
			n := r.buildVirtualNotification(userID, v, true, *virtualID)
			notifications = append(notifications, n)
			*virtualID--
		}
		rows2.Close()
	}

	// Query 3: Requests where user is listed as a student subject (via request_students)
	rows3, err := r.db.QueryContext(ctx, `
		SELECT r.id, NULL, rt.code, rt.label, r.reason, r.status, COALESCE(r.submitted_at, r.created_at), COALESCE(tp_req.full_name, pp_req.full_name, '')
		FROM requests r
		JOIN request_types rt ON rt.id = r.request_type_id
		JOIN request_students rs ON rs.request_id = r.id
		JOIN student_profiles sp ON sp.id = rs.student_id
		LEFT JOIN teacher_profiles tp_req ON tp_req.user_id = r.requester_user_id AND tp_req.deleted_at IS NULL
		LEFT JOIN principal_profiles pp_req ON pp_req.user_id = r.requester_user_id
		WHERE sp.user_id = ? AND r.deleted_at IS NULL AND r.status != 'draft'
	`, userID)
	if err == nil {
		defer rows3.Close()
		for rows3.Next() {
			var v virtualRow
			if err := rows3.Scan(&v.RequestID, &v.ApprovalID, &v.ReqTypeCode, &v.ReqTypeLabel, &v.Reason, &v.ReqStatus, &v.CreatedAt, &v.RequesterName); err != nil {
				continue
			}
			if existingReqIDs[v.RequestID] {
				continue
			}
			n := r.buildVirtualNotification(userID, v, false, *virtualID)
			notifications = append(notifications, n)
			*virtualID--
		}
		rows3.Close()
	}

	return notifications, nil
}

func (r *notificationRepository) buildVirtualNotification(userID int64, v virtualRow, isApprover bool, virtualID int64) domain.Notification {
	notifType := "new_request"
	title := fmt.Sprintf("Permohonan %s", v.ReqTypeLabel)
	var body *string

	if isApprover && v.ReqStatus != "pending" {
		switch v.ReqStatus {
		case "approved":
			notifType = "request_approved"
			title = fmt.Sprintf("Permohonan %s Disetujui", v.ReqTypeLabel)
		case "rejected":
			notifType = "request_rejected"
			title = fmt.Sprintf("Permohonan %s Ditolak", v.ReqTypeLabel)
		case "cancelled":
			notifType = "request_cancelled"
			title = fmt.Sprintf("Permohonan %s Dibatalkan", v.ReqTypeLabel)
		}
	}

	// Build status label in Indonesian
	statusLabel := ""
	switch v.ReqStatus {
	case "pending":
		statusLabel = "Status saat ini: menunggu persetujuan"
	case "approved":
		statusLabel = "Status saat ini: disetujui"
	case "rejected":
		statusLabel = "Status saat ini: ditolak"
	case "cancelled":
		statusLabel = "Status saat ini: dibatalkan"
	}

	if v.RequesterName != "" {
		bodyStr := fmt.Sprintf("Pengajuan surat dispensasi oleh %s", v.RequesterName)
		if statusLabel != "" {
			s := bodyStr + ", " + statusLabel
			body = &s
		} else {
			body = &bodyStr
		}
	} else {
		if v.Reason != nil && *v.Reason != "" {
			body = v.Reason
		}
		if statusLabel != "" {
			if body != nil {
				s := *body + ", " + statusLabel
				body = &s
			} else {
				body = &statusLabel
			}
		}
	}

	reqID := v.RequestID

	return domain.Notification{
		ID:         virtualID,
		UserID:     userID,
		RequestID:  &reqID,
		ApprovalID: v.ApprovalID,
		Type:       notifType,
		Title:      title,
		Body:       body,
		IsRead:     true,
		CreatedAt:  v.CreatedAt,
	}
}

func (r *notificationRepository) Create(ctx context.Context, userID int64, notifType, title string, body *string, requestID, approvalID *int64) error {
	// Ensure the notification type exists in ref_values so the DB trigger
	// trg_notifications_validate_type doesn't reject the INSERT.
	_, _ = r.db.ExecContext(ctx,
		`INSERT IGNORE INTO ref_values (group_key, value, label, description, color, icon, sort_order, is_active)
		 VALUES ('notification_type', ?, ?, 'Auto-registered notification type', 'blue', 'bell', 99, 1)`,
		notifType, title,
	)

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO notifications (user_id, request_id, approval_id, type, title, body)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		userID, nullableInt64(requestID), nullableInt64(approvalID), notifType, title, nullableString(body),
	)
	return err
}

func createNotificationTx(tx *sql.Tx, userID int64, notifType, title string, body *string, requestID, approvalID *int64) error {
	// Same auto-registration for the transactional path.
	_, _ = tx.Exec(
		`INSERT IGNORE INTO ref_values (group_key, value, label, description, color, icon, sort_order, is_active)
		 VALUES ('notification_type', ?, ?, 'Auto-registered notification type', 'blue', 'bell', 99, 1)`,
		notifType, title,
	)

	_, err := tx.Exec(
		`INSERT INTO notifications (user_id, request_id, approval_id, type, title, body)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		userID, nullableInt64(requestID), nullableInt64(approvalID), notifType, title, nullableString(body),
	)
	return err
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func (r *notificationRepository) MarkAsRead(ctx context.Context, notificationID, userID int64) error {
	res, err := r.db.ExecContext(ctx, `UPDATE notifications SET is_read = 1, read_at = NOW() WHERE id = ? AND user_id = ?`, notificationID, userID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
