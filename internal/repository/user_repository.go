package repository

import "github.com/Refliqx/backend-eletter/internal/domain"

type UserRepository interface {
	Create(user domain.User) error
	IsEmailExists(email string) bool
}
