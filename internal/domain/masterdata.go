package domain

import "time"

type Class struct {
	ID             int64      `json:"id"`
	AcademicYearID int64      `json:"academicYearId"`
	MajorID        int64      `json:"majorId"`
	GradeLevel     int        `json:"gradeLevel"`
	ClassName      string     `json:"className"`
	IsActive       bool       `json:"isActive"`
	CreatedAt      *time.Time `json:"createdAt,omitempty"`
	UpdatedAt      *time.Time `json:"updatedAt,omitempty"`
}

type Major struct {
	ID        int64      `json:"id"`
	Code      string     `json:"code"`
	Name      string     `json:"name"`
	IsActive  bool       `json:"isActive"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
}

type StudentProfile struct {
	ID           int64      `json:"id"`
	UserID       int64      `json:"userId"`
	StudentCode  *string    `json:"studentCode,omitempty"`
	FullName     string     `json:"fullName"`
	Gender       *string    `json:"gender,omitempty"`
	BirthDate    *string    `json:"birthDate,omitempty"`
	Phone        *string    `json:"phone,omitempty"`
	SignatureURL *string    `json:"signatureUrl,omitempty"`
	Active       bool       `json:"active"`
	ClassID      *int64     `json:"classId,omitempty"`
	ClassName    *string    `json:"className,omitempty"`
	MajorName    *string    `json:"majorName,omitempty"`
	CreatedAt    *time.Time `json:"createdAt,omitempty"`
	UpdatedAt    *time.Time `json:"updatedAt,omitempty"`
}

type Notification struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"userId"`
	RequestID  *int64     `json:"requestId,omitempty"`
	ApprovalID *int64     `json:"approvalId,omitempty"`
	Type       string     `json:"type"`
	Title      string     `json:"title"`
	Body       *string    `json:"body,omitempty"`
	IsRead     bool       `json:"isRead"`
	ReadAt     *time.Time `json:"readAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
}
