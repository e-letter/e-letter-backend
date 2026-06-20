package repository

import (
	"database/sql"

	"github.com/Refliqx/backend-eletter/internal/domain"
)

type AuthRepository interface {
	domain.AuthRepository
}

type authRepository struct {
	db *sql.DB
}

func NewAuthRepository(db *sql.DB) AuthRepository {
	return &authRepository{db: db}
}
