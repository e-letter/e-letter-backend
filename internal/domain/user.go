package domain

type User struct {
	ID                   int     `json:"id" db:"id"`
	LoginCode            string  `json:"login_code" db:"login_code"`
	RoleID               int     `json:"role_id" db:"role_id"`
	Email                string  `json:"email" db:"email"`
	PasswordHash         string  `json:"-" db:"password_hash"`
	FullName             *string `json:"full_name,omitempty" db:"full_name"`
	NISN                 *string `json:"nisn,omitempty" db:"nisn"`
	NIP                  *string `json:"nip,omitempty" db:"nip"`
	Gender               *string `json:"gender,omitempty" db:"gender"`
	PhoneNumber          *string `json:"phone_number,omitempty" db:"phone_number"`
	ClassID              *int    `json:"class_id,omitempty" db:"class_id"`
	CanRequestDispensasi bool    `json:"can_request_dispensasi" db:"can_request_dispensasi"`
	ProfileCompleted     bool    `json:"profile_completed" db:"profile_completed"`
	IsActive             bool    `json:"is_active" db:"is_active"`
	CreatedAt            string  `json:"created_at" db:"created_at"`
	UpdatedAt            string  `json:"updated_at" db:"updated_at"`
}
