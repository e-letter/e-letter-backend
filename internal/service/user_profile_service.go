package service

import (
	"errors"

	"github.com/Refliqx/backend-eletter/internal/domain"
	"github.com/Refliqx/backend-eletter/internal/repository"
)

type UserProfileService interface {
	domain.UserProfileService
}

type userProfileService struct {
	repo repository.UserProfileRepository
}

func NewUserProfileService(repo repository.UserProfileRepository) UserProfileService {
	return &userProfileService{repo: repo}
}

func (s *userProfileService) GetProfile(userID int) (*domain.User, error) {
	if userID <= 0 {
		return nil, errors.New("Token tidak valid")
	}
	return s.repo.GetByUserID(userID)
}

func (s *userProfileService) UpdateProfile(req domain.UserProfileUpdatePayload) (*domain.User, error) {
	if req.UserID <= 0 {
		return nil, errors.New("Missing userId")
	}
	return s.repo.Update(req.UserID, req.UserProfileUpdateRequest)
}
