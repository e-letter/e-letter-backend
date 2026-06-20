package service

import (
	"context"

	"github.com/Refliqx/backend-eletter/internal/domain"
	"github.com/Refliqx/backend-eletter/internal/repository"
)

type MasterDataService interface {
	GetAllClasses(ctx context.Context) ([]domain.Class, error)
	GetClassByID(ctx context.Context, id int64) (*domain.Class, error)
	GetAllMajors(ctx context.Context) ([]domain.Major, error)
	GetMajorByID(ctx context.Context, id int64) (*domain.Major, error)
	GetStudents(ctx context.Context, classID, majorID *int64) ([]domain.StudentProfile, error)
}

type masterDataService struct {
	repo repository.MasterDataRepository
}

func NewMasterDataService(repo repository.MasterDataRepository) MasterDataService {
	return &masterDataService{repo: repo}
}

func (s *masterDataService) GetAllClasses(ctx context.Context) ([]domain.Class, error) {
	return s.repo.GetAllClasses(ctx)
}

func (s *masterDataService) GetClassByID(ctx context.Context, id int64) (*domain.Class, error) {
	return s.repo.GetClassByID(ctx, id)
}

func (s *masterDataService) GetAllMajors(ctx context.Context) ([]domain.Major, error) {
	return s.repo.GetAllMajors(ctx)
}

func (s *masterDataService) GetMajorByID(ctx context.Context, id int64) (*domain.Major, error) {
	return s.repo.GetMajorByID(ctx, id)
}

func (s *masterDataService) GetStudents(ctx context.Context, classID, majorID *int64) ([]domain.StudentProfile, error) {
	return s.repo.GetStudents(ctx, classID, majorID)
}
