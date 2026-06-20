package repository

import (
	"database/sql"

	"github.com/Refliqx/backend-eletter/internal/domain"
)

type AttachmentRepository interface {
	domain.AttachmentRepository
}

type attachmentRepository struct {
	db *sql.DB
}

func NewAttachmentRepository(db *sql.DB) AttachmentRepository {
	return &attachmentRepository{db: db}
}

func (r *attachmentRepository) GetByID(id int) (*domain.Attachment, error) {
	row := r.db.QueryRow(`SELECT id, request_id, file_path, original_name, mime_type, file_size FROM request_attachments WHERE id = ?`, id)
	var a domain.Attachment
	if err := row.Scan(&a.ID, &a.RequestID, &a.FilePath, &a.OriginalName, &a.MimeType, &a.FileSize); err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *attachmentRepository) GetByRequestID(requestID int) ([]domain.Attachment, error) {
	rows, err := r.db.Query(`SELECT id, request_id, file_path, original_name, mime_type, file_size FROM request_attachments WHERE request_id = ? ORDER BY created_at DESC`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Attachment, 0)
	for rows.Next() {
		var a domain.Attachment
		if err := rows.Scan(&a.ID, &a.RequestID, &a.FilePath, &a.OriginalName, &a.MimeType, &a.FileSize); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *attachmentRepository) Create(requestID int, filePath, originalName, mimeType string, fileSize int64) (int, error) {
	res, err := r.db.Exec(`INSERT INTO request_attachments (request_id, file_path, original_name, mime_type, file_size) VALUES (?, ?, ?, ?, ?)`,
		requestID, filePath, originalName, mimeType, fileSize)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}
