package domain

import "time"

type PermissionRequest struct {
	RequestID     int        `json:"request_id" db:"request_id"`
	TypeID        int        `json:"type_id" db:"type_id"`
	NoSurat       *string    `json:"no_surat,omitempty" db:"no_surat"`
	CreatedBy     int        `json:"created_by" db:"created_by"`
	Title         *string    `json:"title,omitempty" db:"title"`
	Description   *string    `json:"description,omitempty" db:"description"`
	EventLocation *string    `json:"event_location,omitempty" db:"event_location"`
	StartTime     *time.Time `json:"start_time,omitempty" db:"start_time"`
	EndTime       *time.Time `json:"end_time,omitempty" db:"end_time"`
	Signature     *string    `json:"signature,omitempty" db:"signature"`
	Status        string     `json:"status" db:"status"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" db:"updated_at"`
}

type PermissionClass struct {
	ClassID   int    `json:"class_id" db:"class_id"`
	ClassName string `json:"class_name" db:"class_name"`
}

type PermissionMajor struct {
	MajorID    int    `json:"major_id" db:"major_id"`
	MajorName  string `json:"major_name" db:"major_name"`
	MajorShort string `json:"major_short" db:"major_short"`
}

type ApprovalRequest struct {
	RequestID    int     `json:"request_id"`
	StageID      int     `json:"stage_id"`
	Status       string  `json:"status"`
	Notes        *string `json:"notes"`
	SignatureURL *string `json:"signature_url"`
}

type CreatePermissionRequest struct {
	IDSiswa      int     `json:"id_siswa"`
	TypeID       int     `json:"type_id"`
	Title        *string `json:"title"`
	Description  *string `json:"description"`
	EventLocation *string `json:"event_location"`
	StartDate    string  `json:"start_date"`
	EndDate      string  `json:"end_date"`
}

type UpdatePermissionRequest struct {
	RequestID     int     `json:"request_id"`
	Title         *string `json:"title"`
	Description   *string `json:"description"`
	EventLocation *string `json:"event_location"`
	StartTime     *string `json:"start_time"`
	EndTime       *string `json:"end_time"`
	Status        *string `json:"status"`
}

type PermissionRepository interface {
	ListAll() ([]PermissionRequest, error)
	ListByUser(userID int) ([]PermissionRequest, error)
	ListClasses() ([]PermissionClass, error)
	ListMajors() ([]PermissionMajor, error)
	GetUserByNISN(nisn string) (*User, error)
	GetUserByID(userID int) (*User, error)
	Create(req CreatePermissionRequest) (int, error)
	Update(req UpdatePermissionRequest) error
	Delete(requestID int) error
	Approve(req ApprovalRequest, approverID int) error
	ListRegistrationTokens() ([]TokenRecord, error)
	CreateOrUpdateRegistrationToken(token string, roleID int, usageLimit *int, expiresAt *time.Time) error
	GetRegistrationTokenByValue(token string) (*TokenRecord, error)
}

type PermissionService interface {
	Get(action, idSiswa, nisn string, userID int, roleID int) (any, error)
	Create(req CreatePermissionRequest) (int, error)
	Update(req UpdatePermissionRequest) error
	Delete(id int) error
	Approve(req ApprovalRequest, approverID int) error
	ListRegistrationTokens() ([]TokenRecord, error)
	UpsertRegistrationToken(token string, roleID int, usageLimit *int, expiresAt *time.Time) (*TokenRecord, error)
}
