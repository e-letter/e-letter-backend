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

func (s *permissionService) Get(action, idSiswa, nisn string, userID int, roleID int, startDate, endDate, search, status, typeKey string, page, limit int) (any, error) {
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
		return s.repo.ListByUser(uid, startDate, endDate, search, status, typeKey, page, limit)
	}
	if nisn != "" {
		u, err := s.repo.GetUserByNISN(nisn)
		if err != nil {
			return nil, errors.New("User not found")
		}
		return s.repo.ListByUser(u.ID, startDate, endDate, search, status, typeKey, page, limit)
	}

	if userID <= 0 {
		return nil, errors.New("Token akses diperlukan")
	}

	user, err := s.repo.GetUserByID(userID)
	if err != nil || user == nil {
		return nil, errors.New("User tidak ditemukan")
	}
	if roleID == 2 || user.Role == "admin" {
		return s.repo.ListAll(startDate, endDate, search, status, typeKey, page, limit)
	}
	return s.repo.ListByUser(userID, startDate, endDate, search, status, typeKey, page, limit)
}

func (s *permissionService) Create(req domain.CreatePermissionRequest) (int, error) {
	if req.IDSiswa == 0 || req.TypeID == 0 || req.RequestDate == "" || req.StartDate == "" || req.EndDate == "" {
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
	if req.Status != "approved" && req.Status != "rejected" && req.Status != "skipped" {
		return errors.New("invalid status: must be approved, rejected, or skipped")
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

func (s *permissionService) CancelRequest(requestID, userID int, reason string) error {
	return s.repo.CancelRequest(requestID, userID, reason)
}

func (s *permissionService) GetRequestDetail(requestID int) (any, error) {
	return s.repo.GetRequestDetail(requestID)
}

func (s *permissionService) GetTeacherRoles(userID int) (any, error) {
	return s.repo.GetTeacherRoles(userID)
}

func (s *permissionService) RequestTeacherRole(userID int, roleName string, meta domain.TeacherRoleMetadata) error {
	return s.repo.RequestTeacherRole(userID, roleName, meta)
}

func (s *permissionService) CreateDelegation(userID, delegateUserID int, validFrom, validUntil, reason string) error {
	return s.repo.CreateDelegation(userID, delegateUserID, validFrom, validUntil, reason)
}

func (s *permissionService) ListDelegations(userID int) (any, error) {
	return s.repo.ListDelegations(userID)
}

func (s *permissionService) DeleteDelegation(id, userID int) error {
	return s.repo.DeleteDelegation(id, userID)
}
