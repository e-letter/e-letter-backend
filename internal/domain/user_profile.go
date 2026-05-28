package domain

type UserProfileRepository interface {
	GetByUserID(userID int) (*User, error)
	Update(userID int, payload UserProfileUpdateRequest) (*User, error)
	CompleteTeacherOnboarding(payload CompleteTeacherOnboardingPayload) (*User, error)
}

type UserProfileService interface {
	GetProfile(userID int) (*User, error)
	UpdateProfile(req UserProfileUpdatePayload) (*User, error)
	CompleteTeacherOnboarding(payload CompleteTeacherOnboardingPayload) (*User, error)
}

type UserProfileUpdateRequest struct {
	FullName          *string `json:"full_name"`
	Email             *string `json:"email"`
	PhoneNumber       *string `json:"phone_number"`
	Gender            *string `json:"gender"`
	NISN              *string `json:"nisn"`
	NIP               *string `json:"nip"`
	SchoolName        *string `json:"school_name"`
	SignatureUrl      *string `json:"signature_url"`
	ClassID           *int    `json:"class_id"`
	MarkProfileFinish bool    `json:"markProfileCompleted"`
}

type UserProfileUpdatePayload struct {
	UserID int `json:"userId"`
	UserProfileUpdateRequest
}

type ScheduleDetail struct {
	ClassID   int    `json:"classId"`
	SubjectID int    `json:"subjectId"`
	DayOfWeek string `json:"dayOfWeek"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
}

type CompleteTeacherOnboardingPayload struct {
	UserID          int              `json:"userId"`
	FullName        string           `json:"fullName"`
	NIP             string           `json:"nip"`
	Gender          string           `json:"gender"`
	SignatureUrl    string           `json:"signatureUrl"`
	Roles           []string         `json:"roles"`
	HomeroomClassID int              `json:"homeroomClassId"`
	MajorID         int              `json:"majorId"`
	Subjects        []int            `json:"subjects"`
	Schedules       []ScheduleDetail `json:"schedules"`
}
