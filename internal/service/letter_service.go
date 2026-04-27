package service

import (
	"errors"
	"strings"

	"github.com/Refliqx/backend-eletter/internal/domain"
	"github.com/Refliqx/backend-eletter/internal/repository"
)

type LetterService interface {
	domain.LetterService
}

type letterService struct {
	repo repository.LetterRepository
}

func NewLetterService(repo repository.LetterRepository) LetterService {
	return &letterService{repo: repo}
}

func (s *letterService) Create(userID int, req domain.LetterCreateRequest) (int, error) {
	if req.TypeID == 0 || strings.TrimSpace(req.StartTime) == "" || strings.TrimSpace(req.EndTime) == "" {
		return 0, errors.New("type_id, start_time, dan end_time diperlukan")
	}
	return s.repo.CreateLetter(userID, req)
}

func (s *letterService) ListForStudent(userID int, typeKey string) ([]domain.LetterListItem, error) {
	return s.repo.ListLettersForUser(userID, typeKey)
}

func (s *letterService) ListForTeacher(typeKey string) ([]domain.LetterListItem, error) {
	return s.repo.ListLettersForTeacher(typeKey)
}
