package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Refliqx/backend-eletter/internal/domain"
)

type MasterDataRepository interface {
	GetAllClasses(ctx context.Context) ([]domain.Class, error)
	GetClassByID(ctx context.Context, id int64) (*domain.Class, error)
	GetAllMajors(ctx context.Context) ([]domain.Major, error)
	GetMajorByID(ctx context.Context, id int64) (*domain.Major, error)
	GetStudentsByClass(ctx context.Context, classID int64) ([]domain.StudentProfile, error)
	GetStudentByID(ctx context.Context, id int64) (*domain.StudentProfile, error)
	GetStudents(ctx context.Context, classID, majorID *int64) ([]domain.StudentProfile, error)
}

type masterDataRepository struct {
	db *sql.DB
}

func NewMasterDataRepository(db *sql.DB) MasterDataRepository {
	return &masterDataRepository{db: db}
}

func (r *masterDataRepository) GetAllClasses(ctx context.Context) ([]domain.Class, error) {
	query := `
		SELECT id, academic_year_id, major_id, grade_level, class_name, is_active, created_at, updated_at
		FROM classes
		WHERE is_active = 1
		ORDER BY grade_level, class_name
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query classes error: %w", err)
	}
	defer rows.Close()

	var classes []domain.Class
	for rows.Next() {
		var c domain.Class
		if err := rows.Scan(&c.ID, &c.AcademicYearID, &c.MajorID, &c.GradeLevel, &c.ClassName, &c.IsActive, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan class error: %w", err)
		}
		classes = append(classes, c)
	}

	return classes, rows.Err()
}

func (r *masterDataRepository) GetClassByID(ctx context.Context, id int64) (*domain.Class, error) {
	query := `
		SELECT id, academic_year_id, major_id, grade_level, class_name, is_active, created_at, updated_at
		FROM classes
		WHERE id = ? AND is_active = 1
	`

	var c domain.Class
	if err := r.db.QueryRowContext(ctx, query, id).Scan(&c.ID, &c.AcademicYearID, &c.MajorID, &c.GradeLevel, &c.ClassName, &c.IsActive, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query class error: %w", err)
	}

	return &c, nil
}

func (r *masterDataRepository) GetAllMajors(ctx context.Context) ([]domain.Major, error) {
	query := `
		SELECT id, code, name, is_active, created_at, updated_at
		FROM majors
		WHERE is_active = 1
		ORDER BY name
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query majors error: %w", err)
	}
	defer rows.Close()

	var majors []domain.Major
	for rows.Next() {
		var m domain.Major
		if err := rows.Scan(&m.ID, &m.Code, &m.Name, &m.IsActive, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan major error: %w", err)
		}
		majors = append(majors, m)
	}

	return majors, rows.Err()
}

func (r *masterDataRepository) GetMajorByID(ctx context.Context, id int64) (*domain.Major, error) {
	query := `
		SELECT id, code, name, is_active, created_at, updated_at
		FROM majors
		WHERE id = ? AND is_active = 1
	`

	var m domain.Major
	if err := r.db.QueryRowContext(ctx, query, id).Scan(&m.ID, &m.Code, &m.Name, &m.IsActive, &m.CreatedAt, &m.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query major error: %w", err)
	}

	return &m, nil
}

func (r *masterDataRepository) GetStudentsByClass(ctx context.Context, classID int64) ([]domain.StudentProfile, error) {
	query := `
		SELECT sp.id, sp.user_id, sp.student_code, sp.full_name, sp.gender, sp.birth_date,
		       sp.phone, sp.signature_url, sp.active, sce.class_id, c.class_name, m.name,
		       sp.created_at, sp.updated_at
		FROM student_profiles sp
		JOIN student_class_enrollments sce ON sce.student_id = sp.id AND sce.is_active = 1
		JOIN classes c ON sce.class_id = c.id
		LEFT JOIN majors m ON c.major_id = m.id
		WHERE sce.class_id = ? AND sp.deleted_at IS NULL
		ORDER BY sp.full_name
	`

	rows, err := r.db.QueryContext(ctx, query, classID)
	if err != nil {
		return nil, fmt.Errorf("query students error: %w", err)
	}
	defer rows.Close()

	var students []domain.StudentProfile
	for rows.Next() {
		var s domain.StudentProfile
		if err := rows.Scan(&s.ID, &s.UserID, &s.StudentCode, &s.FullName, &s.Gender, &s.BirthDate,
			&s.Phone, &s.SignatureURL, &s.Active, &s.ClassID, &s.ClassName, &s.MajorName,
			&s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan student error: %w", err)
		}
		students = append(students, s)
	}

	return students, rows.Err()
}

func (r *masterDataRepository) GetStudentByID(ctx context.Context, id int64) (*domain.StudentProfile, error) {
	query := `
		SELECT sp.id, sp.user_id, sp.student_code, sp.full_name, sp.gender, sp.birth_date,
		       sp.phone, sp.signature_url, sp.active,
		       (SELECT sce.class_id FROM student_class_enrollments sce WHERE sce.student_id = sp.id AND sce.is_active = 1 LIMIT 1) as class_id,
		       (SELECT c.class_name FROM student_class_enrollments sce2 JOIN classes c ON c.id = sce2.class_id WHERE sce2.student_id = sp.id AND sce2.is_active = 1 LIMIT 1) as class_name,
		       (SELECT m.name FROM student_class_enrollments sce3 JOIN classes c2 ON c2.id = sce3.class_id LEFT JOIN majors m ON m.id = c2.major_id WHERE sce3.student_id = sp.id AND sce3.is_active = 1 LIMIT 1) as major_name,
		       sp.created_at, sp.updated_at
		FROM student_profiles sp
		WHERE sp.id = ? AND sp.deleted_at IS NULL
	`

	var s domain.StudentProfile
	if err := r.db.QueryRowContext(ctx, query, id).Scan(&s.ID, &s.UserID, &s.StudentCode, &s.FullName, &s.Gender, &s.BirthDate,
		&s.Phone, &s.SignatureURL, &s.Active, &s.ClassID, &s.ClassName, &s.MajorName,
		&s.CreatedAt, &s.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query student error: %w", err)
	}

	return &s, nil
}

func (r *masterDataRepository) GetStudents(ctx context.Context, classID, majorID *int64) ([]domain.StudentProfile, error) {
	query := `
		SELECT sp.id, sp.user_id, sp.student_code, sp.full_name, sp.gender, sp.birth_date,
		       sp.phone, sp.signature_url, sp.active, sce.class_id, c.class_name, m.name,
		       sp.created_at, sp.updated_at
		FROM student_profiles sp
		JOIN student_class_enrollments sce ON sce.student_id = sp.id AND sce.is_active = 1
		JOIN classes c ON sce.class_id = c.id
		LEFT JOIN majors m ON c.major_id = m.id
		WHERE c.is_active = 1 AND sp.deleted_at IS NULL
	`
	args := []interface{}{}

	if classID != nil {
		query += " AND sce.class_id = ?"
		args = append(args, *classID)
	}
	if majorID != nil {
		query += " AND c.major_id = ?"
		args = append(args, *majorID)
	}

	query += " ORDER BY sp.full_name"

	var rows *sql.Rows
	var err error
	if len(args) > 0 {
		rows, err = r.db.QueryContext(ctx, query, args...)
	} else {
		rows, err = r.db.QueryContext(ctx, query)
	}
	if err != nil {
		return nil, fmt.Errorf("query students error: %w", err)
	}
	defer rows.Close()

	var students []domain.StudentProfile
	for rows.Next() {
		var s domain.StudentProfile
		if err := rows.Scan(&s.ID, &s.UserID, &s.StudentCode, &s.FullName, &s.Gender, &s.BirthDate,
			&s.Phone, &s.SignatureURL, &s.Active, &s.ClassID, &s.ClassName, &s.MajorName,
			&s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan student error: %w", err)
		}
		students = append(students, s)
	}

	return students, rows.Err()
}
