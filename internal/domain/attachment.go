package domain

type Attachment struct {
	ID           int     `json:"id" db:"id"`
	RequestID    int     `json:"request_id" db:"request_id"`
	FilePath     string  `json:"file_path" db:"file_path"`
	OriginalName string  `json:"original_name" db:"original_name"`
	MimeType     *string `json:"mime_type,omitempty" db:"mime_type"`
	FileSize     *int64  `json:"file_size,omitempty" db:"file_size"`
}

type AttachmentRepository interface {
	GetByID(id int) (*Attachment, error)
	GetByRequestID(requestID int) ([]Attachment, error)
	Create(requestID int, filePath, originalName, mimeType string, fileSize int64) (int, error)
}

type AttachmentService interface {
	GetByID(id int) (*Attachment, error)
	GetByRequestID(requestID int) ([]Attachment, error)
	Create(requestID int, filePath, originalName, mimeType string, fileSize int64) (int, error)
}
