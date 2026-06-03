package service

import (
	"fmt"

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
	return s.repo.GetByUserID(userID)
}

func (s *userProfileService) UpdateProfile(req domain.UserProfileUpdatePayload) (*domain.User, error) {
	// Admin users (userID=0) don't need profile updates
	if req.UserID == 0 {
		return s.repo.GetByUserID(0)
	}
	if req.UserID < 0 {
		return nil, fmt.Errorf("Missing userId")
	}
	return s.repo.Update(req.UserID, req.UserProfileUpdateRequest)
}

func (s *userProfileService) CompleteTeacherOnboarding(payload domain.CompleteTeacherOnboardingPayload) (*domain.User, error) {
	if payload.UserID <= 0 {
		return nil, fmt.Errorf("Missing or invalid userId")
	}
	return s.repo.CompleteTeacherOnboarding(payload)
}

func (s *userProfileService) GetSchedules(userID int) ([]domain.ScheduleDetail, error) {
	return s.repo.GetSchedules(userID)
}
