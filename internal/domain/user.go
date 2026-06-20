package domain

import "time"

type User struct {
	ID       int     `json:"id" db:"id"`
	Username *string `json:"username,omitempty" db:"username"`
	Email    *string `json:"email,omitempty" db:"email"`
	Role     string  `json:"role" db:"role"`
	Status   string  `json:"status" db:"status"`

	FullName             *string    `json:"full_name,omitempty"`
	StudentCode          *string    `json:"student_code,omitempty"`
	EmployeeCode         *string    `json:"employee_code,omitempty"`
	Gender               *string    `json:"gender,omitempty"`
	PhoneNumber          *string    `json:"phone_number,omitempty"`
	ClassID              *int       `json:"class_id,omitempty"`
	CanRequestDispensasi bool       `json:"can_request_dispensasi"`
	ProfileCompleted     bool       `json:"profile_completed"`
	SignatureURL         *string    `json:"signature_url,omitempty"`
	CreatedAt            *time.Time `json:"created_at,omitempty"`
	UpdatedAt            *time.Time `json:"updated_at,omitempty"`
	PasswordChangedAt    *time.Time `json:"password_changed_at,omitempty"`

	PasswordHash string `json:"-" db:"password_hash"`
}
