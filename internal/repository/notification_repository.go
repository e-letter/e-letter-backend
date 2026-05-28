package repository

import (
	"context"
	"database/sql"

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

	return notifications, rows.Err()
}

func (r *notificationRepository) Create(ctx context.Context, userID int64, notifType, title string, body *string, requestID, approvalID *int64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO notifications (user_id, request_id, approval_id, type, title, body)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		userID, nullableInt64(requestID), nullableInt64(approvalID), notifType, title, nullableString(body),
	)
	return err
}

func createNotificationTx(tx *sql.Tx, userID int64, notifType, title string, body *string, requestID, approvalID *int64) error {
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
