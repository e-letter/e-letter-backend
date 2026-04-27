package repository

import (
	"database/sql"
	"time"

	"github.com/Refliqx/backend-eletter/internal/domain"
)

type LetterRepository interface {
	domain.LetterRepository
}

type letterRepository struct {
	db *sql.DB
}

func NewLetterRepository(db *sql.DB) LetterRepository {
	return &letterRepository{db: db}
}

func (r *letterRepository) CreateLetter(userID int, req domain.LetterCreateRequest) (int, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var requestID int
	if err := tx.QueryRow(
		`INSERT INTO permission_requests (type_id, created_by, no_surat, title, description, event_location, start_time, end_time, signature, status, is_active, request_date)
		 VALUES ($1,$2,NULL,$3,$4,NULL,$5,$6,$7,'PENDING',true,NOW()) RETURNING request_id`,
		req.TypeID, userID, req.Title, req.Description, req.StartTime, req.EndTime, req.SignatureURL,
	).Scan(&requestID); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return requestID, nil
}

func (r *letterRepository) ListLettersForUser(userID int, typeKey string) ([]domain.LetterListItem, error) {
	rows, err := r.db.Query(`
		SELECT pr.request_id, pr.title, pr.description, pr.status, pr.created_at, pr.updated_at, pr.start_time, pr.end_time,
		       COALESCE(up.full_name,''), COALESCE(c.class_name,'-'), COALESCE(up.nisn,'-'), COALESCE(u.email,'-')
		FROM permission_requests pr
		JOIN permission_types pt ON pt.type_id = pr.type_id
		JOIN users u ON u.id = pr.created_by
		LEFT JOIN user_profiles up ON up.user_id = u.id
		LEFT JOIN classes c ON c.class_id = up.class_id
		WHERE pr.created_by = $1 AND pt.type_key = $2
		ORDER BY pr.created_at DESC
	`, userID, typeKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLetterRows(rows)
}

func (r *letterRepository) ListLettersForTeacher(typeKey string) ([]domain.LetterListItem, error) {
	rows, err := r.db.Query(`
		SELECT pr.request_id, pr.title, pr.description, pr.status, pr.created_at, pr.updated_at, pr.start_time, pr.end_time,
		       COALESCE(up.full_name,''), COALESCE(c.class_name,'-'), COALESCE(up.nisn,'-'), COALESCE(u.email,'-')
		FROM permission_requests pr
		JOIN permission_types pt ON pt.type_id = pr.type_id
		JOIN users u ON u.id = pr.created_by
		LEFT JOIN user_profiles up ON up.user_id = u.id
		LEFT JOIN classes c ON c.class_id = up.class_id
		WHERE pt.type_key = $1
		ORDER BY pr.created_at DESC
	`, typeKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLetterRows(rows)
}

func scanLetterRows(rows *sql.Rows) ([]domain.LetterListItem, error) {
	items := make([]domain.LetterListItem, 0)
	for rows.Next() {
		var (
			id, title, description, status, fullName, className, nisn, email string
			createdAt, updatedAt                                              time.Time
			startTime, endTime                                                sql.NullTime
		)
		var idInt int
		if err := rows.Scan(&idInt, &title, &description, &status, &createdAt, &updatedAt, &startTime, &endTime, &fullName, &className, &nisn, &email); err != nil {
			return nil, err
		}
		statusUI := mapStatus(status)
		submitted := createdAt.Format("2006-01-02")
		item := domain.LetterListItem{
			ID:          idInt,
			Title:       coalesceTitle(title),
			Status:      statusUI,
			Date:        submitted,
			Description: description,
			StudentInfo: map[string]any{
				"name":  fullName,
				"class": className,
				"nisn":  nisn,
				"email": email,
			},
			RequestInfo: map[string]any{
				"submittedDate": submitted,
				"approvedDate":  approvedDate(status, updatedAt),
				"notes":         description,
			},
			TimeInfo: map[string]any{
				"startTime": formatNullableTime(startTime),
				"endTime":   formatNullableTime(endTime),
			},
		}
		items = append(items, item)
		_ = id
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func mapStatus(status string) string {
	switch status {
	case "APPROVED":
		return "disetujui"
	case "REJECTED", "CANCELLED":
		return "ditolak"
	default:
		return "menunggu"
	}
}

func approvedDate(status string, updatedAt time.Time) any {
	if status == "APPROVED" {
		return updatedAt.Format("2006-01-02")
	}
	return nil
}

func formatNullableTime(t sql.NullTime) string {
	if !t.Valid {
		return "-"
	}
	return t.Time.Format("15:04:05")
}

func coalesceTitle(v string) string {
	if v == "" {
		return "Surat"
	}
	return v
}
