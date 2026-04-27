package domain

type UserProfileRepository interface {
	GetByUserID(userID int) (*User, error)
	Update(userID int, payload UserProfileUpdateRequest) (*User, error)
}

type UserProfileService interface {
	GetProfile(userID int) (*User, error)
	UpdateProfile(req UserProfileUpdatePayload) (*User, error)
}

type UserProfileUpdateRequest struct {
	FullName          *string `json:"full_name"`
	Email             *string `json:"email"`
	PhoneNumber       *string `json:"phone_number"`
	Gender            *string `json:"gender"`
	NISN              *string `json:"nisn"`
	NIP               *string `json:"nip"`
	SchoolName        *string `json:"school_name"`
	ClassID           *int    `json:"class_id"`
	MarkProfileFinish bool    `json:"markProfileCompleted"`
}

type UserProfileUpdatePayload struct {
	UserID int `json:"userId"`
	UserProfileUpdateRequest
}
