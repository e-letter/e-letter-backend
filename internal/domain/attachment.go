package domain

type Attachment struct {
	ID         int     `json:"id" db:"id"`
	RequestID  int     `json:"request_id" db:"request_id"`
	UploadedBy int     `json:"uploaded_by" db:"uploaded_by"`
	FileURL    string  `json:"file_url" db:"file_url"`
	FileName   *string `json:"file_name" db:"file_name"`
	MimeType   *string `json:"mime_type" db:"mime_type"`
	FileSize   int64   `json:"file_size" db:"file_size"`
}

type AttachmentRepository interface {
	GetByID(id int) (*Attachment, error)
	GetByRequestID(requestID int) ([]Attachment, error)
}

type AttachmentService interface {
	GetByID(id int) (*Attachment, error)
	GetByRequestID(requestID int) ([]Attachment, error)
}
