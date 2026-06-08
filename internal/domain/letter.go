package domain

type LetterCreateRequest struct {
	TypeID       int     `json:"type_id"`
	Title        *string `json:"title"`
	Description  *string `json:"description"`
	RequestDate  string  `json:"request_date"`
	StartTime    string  `json:"start_time"`
	EndTime      string  `json:"end_time"`
	SignatureURL *string `json:"signature_url"`
	Students     []int   `json:"students,omitempty"`
}

type StudentInfoDTO struct {
	Name  string `json:"name"`
	Class string `json:"class"`
	NISN  string `json:"nisn"`
	Email string `json:"email"`
}

type RequestInfoDTO struct {
	SubmittedDate string `json:"submittedDate"`
	ApprovedDate  string `json:"approvedDate"`
	Notes         string `json:"notes"`
}

type TimeInfoDTO struct {
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
}

type LetterListItem struct {
	ID          int              `json:"id"`
	Title       string           `json:"title"`
	Status      string           `json:"status"`
	Date        string           `json:"date"`
	Description string           `json:"description"`
	StudentInfo StudentInfoDTO   `json:"studentInfo"`
	Students    []StudentInfoDTO `json:"students,omitempty"`
	RequestInfo RequestInfoDTO   `json:"requestInfo"`
	TimeInfo    TimeInfoDTO      `json:"timeInfo"`
}

type PaginatedLetterResponse struct {
	Data        []LetterListItem `json:"data"`
	CurrentPage int              `json:"currentPage"`
	TotalPages  int              `json:"totalPages"`
	TotalItems  int              `json:"totalItems"`
}

type TeacherRole struct {
	RoleName string
}

type RequestTypeInfo struct {
	ID            int
	Code          string
	Label         string
	LetterPrefix  string
	RequesterRole string
	DurationDays  int
	IsActive      bool
}

type LetterRepository interface {
	CreateLetter(userID int, req LetterCreateRequest) (int, error)
	ListLettersForUser(userID int, typeKey string, page, limit int) (*PaginatedLetterResponse, error)
	ListLettersForTeacher(typeKey string, page, limit int) (*PaginatedLetterResponse, error)
	ListLettersForTeacherScoped(userID int, typeKey string, page, limit int) (*PaginatedLetterResponse, error)
	ListGeneralDispensasi(userRole string, userID int, page, limit int) (*PaginatedLetterResponse, error)
	ListTeacherLetters(userID int, page, limit int) (*PaginatedLetterResponse, error)
	ListPendingForTeacher(userID int, page, limit int) (*PaginatedLetterResponse, error)
	GetTeacherActiveRoles(userID int) ([]TeacherRole, error)
	IsActivePrincipal(userID int) (bool, error)
	GetUserRole(userID int) (string, error)
	GetRequestTypeInfo(typeID int) (*RequestTypeInfo, error)
	HasActiveRequest(userID int, requestTypeID int, requestDate string) (bool, error)
}

type LetterService interface {
	Create(userID int, req LetterCreateRequest) (int, error)
	ListForStudent(userID int, typeKey string, page, limit int) (*PaginatedLetterResponse, error)
	ListForTeacher(typeKey string, page, limit int) (*PaginatedLetterResponse, error)
	ListForTeacherScoped(userID int, typeKey string, page, limit int) (*PaginatedLetterResponse, error)
	ListGeneralDispensasi(userRole string, userID int, page, limit int) (*PaginatedLetterResponse, error)
	ListTeacherLetters(userID int, page, limit int) (*PaginatedLetterResponse, error)
	ListPendingForTeacher(userID int, page, limit int) (*PaginatedLetterResponse, error)
	GetTeacherStats(userID int) (map[string]any, error)
}
