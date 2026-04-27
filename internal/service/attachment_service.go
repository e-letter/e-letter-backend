package service

import (
	"errors"

	"github.com/Refliqx/backend-eletter/internal/domain"
	"github.com/Refliqx/backend-eletter/internal/repository"
)

type AttachmentService interface {
	domain.AttachmentService
}

type attachmentService struct {
	repo repository.AttachmentRepository
}

func NewAttachmentService(repo repository.AttachmentRepository) AttachmentService {
	return &attachmentService{repo: repo}
}

func (s *attachmentService) GetByID(id int) (*domain.Attachment, error) {
	if id <= 0 {
		return nil, errors.New("Invalid attachment ID")
	}
	return s.repo.GetByID(id)
}

func (s *attachmentService) GetByRequestID(requestID int) ([]domain.Attachment, error) {
	if requestID <= 0 {
		return nil, errors.New("Invalid request ID")
	}
	return s.repo.GetByRequestID(requestID)
}
