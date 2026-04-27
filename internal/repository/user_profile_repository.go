package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/Refliqx/backend-eletter/internal/domain"
)

type UserProfileRepository interface {
	domain.UserProfileRepository
}

type userProfileRepository struct {
	db *sql.DB
}

func NewUserProfileRepository(db *sql.DB) UserProfileRepository {
	return &userProfileRepository{db: db}
}

func (r *userProfileRepository) GetByUserID(userID int) (*domain.User, error) {
	row := r.db.QueryRow(`
		SELECT u.id, COALESCE(u.login_code,''), COALESCE(u.role_id,0), u.email, u.password_hash,
		       up.full_name, up.nisn, up.nip, up.gender, up.phone_number, up.class_id,
		       COALESCE(up.can_request_dispensasi, false), COALESCE(up.profile_completed, false), u.is_active,
		       COALESCE(CAST(u.created_at AS TEXT),''), COALESCE(CAST(u.updated_at AS TEXT),'')
		FROM users u
		LEFT JOIN user_profiles up ON up.user_id = u.id
		WHERE u.id = $1
	`, userID)
	return scanUser(row)
}

func (r *userProfileRepository) Update(userID int, payload domain.UserProfileUpdateRequest) (*domain.User, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if payload.Email != nil {
		if _, err := tx.Exec(`UPDATE users SET email = $1 WHERE id = $2`, *payload.Email, userID); err != nil {
			return nil, err
		}
	}

	updates := []string{}
	args := []any{}
	argPos := 1
	if payload.FullName != nil {
		updates = append(updates, fmt.Sprintf("full_name = $%d", argPos))
		args = append(args, *payload.FullName)
		argPos++
	}
	if payload.PhoneNumber != nil {
		updates = append(updates, fmt.Sprintf("phone_number = $%d", argPos))
		args = append(args, *payload.PhoneNumber)
		argPos++
	}
	if payload.Gender != nil {
		updates = append(updates, fmt.Sprintf("gender = $%d", argPos))
		args = append(args, *payload.Gender)
		argPos++
	}
	if payload.NISN != nil {
		updates = append(updates, fmt.Sprintf("nisn = $%d", argPos))
		args = append(args, *payload.NISN)
		argPos++
	}
	if payload.NIP != nil {
		updates = append(updates, fmt.Sprintf("nip = $%d", argPos))
		args = append(args, *payload.NIP)
		argPos++
	}
	if payload.SchoolName != nil {
		updates = append(updates, fmt.Sprintf("school_name = $%d", argPos))
		args = append(args, *payload.SchoolName)
		argPos++
	}
	if payload.ClassID != nil {
		updates = append(updates, fmt.Sprintf("class_id = $%d", argPos))
		args = append(args, *payload.ClassID)
		argPos++
	}
	if payload.MarkProfileFinish {
		updates = append(updates, "profile_completed = true")
	}

	if len(updates) > 0 {
		args = append(args, userID)
		query := fmt.Sprintf(
			`INSERT INTO user_profiles (user_id, profile_completed) VALUES ($%d, false)
			 ON CONFLICT (user_id) DO UPDATE SET %s`,
			argPos, strings.Join(updates, ", "),
		)
		if _, err := tx.Exec(query, args...); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetByUserID(userID)
}
