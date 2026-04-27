package repository

import (
	"database/sql"

	"github.com/Refliqx/backend-eletter/internal/domain"
)

type userRepository struct {
	db *sql.DB
}

func NewUserRepository() UserRepository {
	return &userRepository{}
}

func (r *userRepository) Create(user domain.User) error {
	query := `INSERT INTO users (email, password_hash) VALUES ($1, $2)`

	_, err := r.db.Exec(query, user.Email, user.PasswordHash)
	return err
}

func (r *userRepository) IsEmailExists(email string) bool {
	var exists bool

	query := `SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)`
	_ = r.db.QueryRow(query, email).Scan(&exists)

	return exists
}
