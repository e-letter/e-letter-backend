package service

import (
	"errors"
	"strconv"
	"time"

	"github.com/Refliqx/backend-eletter/internal/domain"
	"github.com/Refliqx/backend-eletter/internal/repository"
)

type PermissionService interface {
	domain.PermissionService
}

type permissionService struct {
	repo repository.PermissionRepository
}

func NewPermissionService(repo repository.PermissionRepository) PermissionService {
	return &permissionService{repo: repo}
}

func (s *permissionService) Get(action, idSiswa, nisn string, userID int, roleID int) (any, error) {
	if action == "kelas" {
		return s.repo.ListClasses()
	}
	if action == "majors" {
		return s.repo.ListMajors()
	}

	if idSiswa != "" {
		uid, err := strconv.Atoi(idSiswa)
		if err != nil {
			return nil, err
		}
		return s.repo.ListByUser(uid)
	}
	if nisn != "" {
		u, err := s.repo.GetUserByNISN(nisn)
		if err != nil {
			return nil, errors.New("User not found")
		}
		return s.repo.ListByUser(u.ID)
	}

	if userID <= 0 {
		return nil, errors.New("Token akses diperlukan")
	}

	user, err := s.repo.GetUserByID(userID)
	if err != nil || user == nil {
		return nil, errors.New("User tidak ditemukan")
	}
	if roleID == 2 {
		return s.repo.ListAll()
	}
	return s.repo.ListByUser(userID)
}

func (s *permissionService) Create(req domain.CreatePermissionRequest) (int, error) {
	if req.IDSiswa == 0 || req.TypeID == 0 || req.StartDate == "" || req.EndDate == "" {
		return 0, errors.New("Missing required fields")
	}
	return s.repo.Create(req)
}

func (s *permissionService) Update(req domain.UpdatePermissionRequest) error {
	if req.RequestID == 0 {
		return errors.New("request_id is required")
	}
	return s.repo.Update(req)
}

func (s *permissionService) Delete(id int) error {
	if id == 0 {
		return errors.New("id is required")
	}
	return s.repo.Delete(id)
}

func (s *permissionService) Approve(req domain.ApprovalRequest, approverID int) error {
	if req.RequestID == 0 || req.StageID == 0 || req.Status == "" {
		return errors.New("request_id, stage_id, and status are required")
	}
	if req.Status != "APPROVED" && req.Status != "REJECTED" && req.Status != "FORWARDED" {
		return errors.New("Invalid status. Must be APPROVED, REJECTED, or FORWARDED")
	}
	return s.repo.Approve(req, approverID)
}

func (s *permissionService) ListRegistrationTokens() ([]domain.TokenRecord, error) {
	return s.repo.ListRegistrationTokens()
}

func (s *permissionService) UpsertRegistrationToken(token string, roleID int, usageLimit *int, expiresAt *time.Time) (*domain.TokenRecord, error) {
	if token == "" || roleID == 0 {
		return nil, errors.New("Missing token or role_id")
	}
	if err := s.repo.CreateOrUpdateRegistrationToken(token, roleID, usageLimit, expiresAt); err != nil {
		return nil, err
	}
	return s.repo.GetRegistrationTokenByValue(token)
}
