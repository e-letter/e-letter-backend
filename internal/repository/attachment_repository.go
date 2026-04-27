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
	row := r.db.QueryRow(`SELECT id, request_id, uploaded_by, file_url, file_name, mime_type, file_size FROM attachments WHERE id = $1`, id)
	var a domain.Attachment
	if err := row.Scan(&a.ID, &a.RequestID, &a.UploadedBy, &a.FileURL, &a.FileName, &a.MimeType, &a.FileSize); err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *attachmentRepository) GetByRequestID(requestID int) ([]domain.Attachment, error) {
	rows, err := r.db.Query(`SELECT id, request_id, uploaded_by, file_url, file_name, mime_type, file_size FROM attachments WHERE request_id = $1 ORDER BY created_at DESC`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Attachment, 0)
	for rows.Next() {
		var a domain.Attachment
		if err := rows.Scan(&a.ID, &a.RequestID, &a.UploadedBy, &a.FileURL, &a.FileName, &a.MimeType, &a.FileSize); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
