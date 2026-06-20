package domain

type AdminUserListItem struct {
	ID       int     `json:"id"`
	Email    *string `json:"email"`
	Role     string  `json:"role"`
	Status   string  `json:"status"`
	FullName string  `json:"full_name"`
}

type AdminRegistrationToken struct {
	ID         int     `json:"id"`
	Token      string  `json:"token"`
	RoleID     int     `json:"role_id"`
	UsageLimit int     `json:"usage_limit"`
	UsedCount  int     `json:"used_count"`
	ExpiresAt  *string `json:"expires_at"`
	CreatedAt  string  `json:"created_at"`
}

type AdminPendingRole struct {
	ID            int     `json:"id"`
	TeacherID     int     `json:"teacher_id"`
	TeacherUserID int     `json:"teacher_user_id"`
	TeacherName   string  `json:"teacher_name"`
	RoleName      string  `json:"role_name"`
	Status        string  `json:"status"`
	ClassID       *int64  `json:"homeroom_class_id,omitempty"`
	ClassName     *string `json:"homeroom_class,omitempty"`
	MajorID       *int64  `json:"major_id,omitempty"`
	MajorName     *string `json:"major_name,omitempty"`
	SubjectIDs    *string `json:"subject_ids,omitempty"`
	CreatedAt     string  `json:"created_at"`
}

type AcademicYear struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	IsActive  *bool  `json:"is_active"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

type AdminClassItem struct {
	ID         int    `json:"id"`
	ClassName  string `json:"class_name"`
	MajorID    int    `json:"major_id"`
	GradeLevel int    `json:"grade_level"`
	MajorName  string `json:"major_name"`
}

type AdminMajorItem struct {
	ID         int    `json:"id"`
	MajorName  string `json:"major_name"`
	MajorShort string `json:"major_short"`
}

type AdminSubjectItem struct {
	ID          int    `json:"id"`
	SubjectName string `json:"subject_name"`
	SubjectCode string `json:"subject_code"`
}

type AdminScheduleItem struct {
	ID             int    `json:"id"`
	AcademicYearID int    `json:"academic_year_id"`
	ClassID        int    `json:"class_id"`
	SubjectID      int    `json:"subject_id"`
	TeacherID      int    `json:"teacher_id"`
	DayOfWeek      string `json:"day_of_week"`
	StartTime      string `json:"start_time"`
	EndTime        string `json:"end_time"`
	ClassName      string `json:"class_name"`
	SubjectName    string `json:"subject_name"`
	TeacherName    string `json:"teacher_name"`
	IsActive       int    `json:"is_active"`
}

type AdminEnrollmentItem struct {
	ID             int     `json:"id"`
	StudentID      int     `json:"student_id"`
	ClassID        int     `json:"class_id"`
	AcademicYearID int     `json:"academic_year_id"`
	StudentName    string  `json:"student_name"`
	StudentCode    string  `json:"student_code"`
	Notes          *string `json:"notes"`
}

type BulkPromotionItem struct {
	EnrollmentID   int  `json:"enrollment_id"`
	StudentID      int  `json:"student_id"`
	SourceClassID  int  `json:"source_class_id"`
	TargetClassID  int  `json:"target_class_id"`
	AcademicYearID int  `json:"academic_year_id"`
	NaikKelas      bool `json:"naik_kelas"`
}

type AuditLogItem struct {
	ID        int     `json:"id"`
	UserID    *int    `json:"user_id"`
	Action    string  `json:"action"`
	Details   *string `json:"details"`
	IPAddress *string `json:"ip_address"`
	UserAgent *string `json:"user_agent"`
	CreatedAt string  `json:"created_at"`
}

type AdminService interface {
	GetStats() (map[string]int, error)
	GetUsers(role, status, search string, page, pageSize int) ([]AdminUserListItem, int, int, int, error)
	UpdateUserStatus(userID int64, status string, adminUserID int64, ip, userAgent string) error
	UpdateUser(id int64, role, fullName *string, adminUserID int64, ip, userAgent string) (map[string]any, error)
	CreateUser(fullName, email, role, status, password string, adminUserID int64, ip, userAgent string) (map[string]any, error)
	AdminDeleteLetter(id int, adminUserID int64, ip, userAgent string) (map[string]any, error)
	GetRegistrationTokens() ([]AdminRegistrationToken, error)
	CreateRegistrationToken(token string, roleID int, usageLimit int, expiresAt *string, adminUserID int64, ip, userAgent string) error
	DeleteRegistrationToken(id string, adminUserID int64, ip, userAgent string) error
	VerifyTeacherRole(id string, adminUserID int64, ip, userAgent string) error
	ListPendingTeacherRoles(status string, page, limit int) ([]AdminPendingRole, int, int, int, error)
	RejectTeacherRole(id string, adminUserID int64, ip, userAgent string) error
	GetAcademicYears() ([]AcademicYear, error)
	CreateAcademicYear(name, startDate, endDate string, adminUserID int64, ip, userAgent string) error
	UpdateAcademicYear(id string, name *string, isActive *bool, startDate, endDate *string, adminUserID int64, ip, userAgent string) error
	DeleteAcademicYear(id string, adminUserID int64, ip, userAgent string) error
	GetClasses() ([]AdminClassItem, error)
	CreateClass(className string, majorID, gradeLevel, academicYearID int, adminUserID int64, ip, userAgent string) error
	UpdateClass(id, className string, majorID int, adminUserID int64, ip, userAgent string) error
	DeleteClass(id string, adminUserID int64, ip, userAgent string) error
	GetMajors() ([]AdminMajorItem, error)
	CreateMajor(name, short string, adminUserID int64, ip, userAgent string) error
	UpdateMajor(id, name, short string, adminUserID int64, ip, userAgent string) error
	DeleteMajor(id string, adminUserID int64, ip, userAgent string) error
	GetSubjects() ([]AdminSubjectItem, error)
	CreateSubject(name, code string, adminUserID int64, ip, userAgent string) error
	UpdateSubject(id, name, code string, adminUserID int64, ip, userAgent string) error
	DeleteSubject(id string, adminUserID int64, ip, userAgent string) error
	GetSchedules() ([]AdminScheduleItem, error)
	CreateSchedule(academicYearID, classID, subjectID, teacherID int, dayOfWeek, startTime, endTime string, adminUserID int64, ip, userAgent string) error
	UpdateSchedule(id string, academicYearID, classID, subjectID, teacherID int, dayOfWeek, startTime, endTime string, adminUserID int64, ip, userAgent string) error
	DeleteSchedule(id string, adminUserID int64, ip, userAgent string) error
	GetEnrollments(classID, search string, page, limit int) ([]AdminEnrollmentItem, int, int, int, error)
	CreateEnrollment(studentID, classID, academicYearID int, notes *string, adminUserID int64, ip, userAgent string) error
	DeleteEnrollment(id string, adminUserID int64, ip, userAgent string) error
	BulkPromoteStudents(promotions []BulkPromotionItem, adminUserID int64, ip, userAgent string) error
	GetSchoolConfig() (map[string]string, error)
	GetPrincipalConfig() (fullName, signatureURL string)
	UpdateSchoolConfig(values map[string]string, adminUserID int64, ip, userAgent string) error
	UploadConfigImage(configKey, filePath string, adminUserID int64, ip, userAgent string) (string, error)
	GetAuditLogs(activityType, search string, page, limit int) ([]AuditLogItem, int, int, int, map[string]int, error)
}
