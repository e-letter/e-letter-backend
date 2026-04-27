package domain

type LetterCreateRequest struct {
	TypeID       int     `json:"type_id"`
	Title        *string `json:"title"`
	Description  *string `json:"description"`
	StartTime    string  `json:"start_time"`
	EndTime      string  `json:"end_time"`
	SignatureURL *string `json:"signature_url"`
}

type LetterListItem struct {
	ID          int         `json:"id"`
	Title       string      `json:"title"`
	Status      string      `json:"status"`
	Date        string      `json:"date"`
	Description string      `json:"description"`
	StudentInfo interface{} `json:"studentInfo,omitempty"`
	RequestInfo interface{} `json:"requestInfo,omitempty"`
	TimeInfo    interface{} `json:"timeInfo,omitempty"`
}

type LetterRepository interface {
	CreateLetter(userID int, req LetterCreateRequest) (int, error)
	ListLettersForUser(userID int, typeKey string) ([]LetterListItem, error)
	ListLettersForTeacher(typeKey string) ([]LetterListItem, error)
}

type LetterService interface {
	Create(userID int, req LetterCreateRequest) (int, error)
	ListForStudent(userID int, typeKey string) ([]LetterListItem, error)
	ListForTeacher(typeKey string) ([]LetterListItem, error)
}
