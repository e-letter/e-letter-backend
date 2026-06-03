package service

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Refliqx/backend-eletter/internal/domain"
	"github.com/Refliqx/backend-eletter/internal/repository"
)

type LetterService interface {
	domain.LetterService
}

type letterService struct {
	repo    repository.LetterRepository
	baseURL string
}

func NewLetterService(repo repository.LetterRepository, baseURL string) LetterService {
	return &letterService{repo: repo, baseURL: baseURL}
}

func (s *letterService) Create(userID int, req domain.LetterCreateRequest) (int, error) {
	if req.TypeID == 0 || strings.TrimSpace(req.RequestDate) == "" || strings.TrimSpace(req.StartTime) == "" || strings.TrimSpace(req.EndTime) == "" {
		return 0, errors.New("type_id, request_date, start_time, dan end_time diperlukan")
	}

	if req.SignatureURL != nil && strings.HasPrefix(*req.SignatureURL, "data:image/png;base64,") {
		encoded := strings.TrimPrefix(*req.SignatureURL, "data:image/png;base64,")
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return 0, fmt.Errorf("failed to decode signature: %w", err)
		}

		// Generate a unique filename for the letter signature (student role assumed for letter creation)
		filename := fmt.Sprintf("student_%d_%d.png", userID, time.Now().UnixNano())
		filePath := filepath.Join("public", "uploads", "signatures", filename)

		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return 0, fmt.Errorf("failed to create signature directory: %w", err)
		}

		if err := os.WriteFile(filePath, decoded, 0644); err != nil {
			return 0, fmt.Errorf("failed to save signature file: %w", err)
		}

		// Store signature URL using the configured base URL.
		signatureURL := strings.TrimRight(s.baseURL, "/") + "/signatures/" + filename
		req.SignatureURL = &signatureURL
	}

	return s.repo.CreateLetter(userID, req)
}

func (s *letterService) ListForStudent(userID int, typeKey string, page, limit int) (*domain.PaginatedLetterResponse, error) {
	return s.repo.ListLettersForUser(userID, typeKey, page, limit)
}

func (s *letterService) ListForTeacher(typeKey string, page, limit int) (*domain.PaginatedLetterResponse, error) {
	return s.repo.ListLettersForTeacher(typeKey, page, limit)
}

func (s *letterService) ListForTeacherScoped(userID int, typeKey string, page, limit int) (*domain.PaginatedLetterResponse, error) {
	// Kepala Sekolah (principal_profiles) has global read access per docs/RBAC.md §1;
	// skip the teacher_roles check for them since they have no teacher_profiles row.
	isPrincipal, err := s.repo.IsActivePrincipal(userID)
	if err != nil {
		return nil, err
	}
	if !isPrincipal {
		roles, err := s.repo.GetTeacherActiveRoles(userID)
		if err != nil {
			return nil, err
		}
		if len(roles) == 0 {
			return nil, errors.New("forbidden: no active roles")
		}
	}
	return s.repo.ListLettersForTeacherScoped(userID, typeKey, page, limit)
}

func (s *letterService) ListPendingForTeacher(userID int, page, limit int) (*domain.PaginatedLetterResponse, error) {
	isPrincipal, err := s.repo.IsActivePrincipal(userID)
	if err != nil {
		return nil, err
	}
	if !isPrincipal {
		roles, err := s.repo.GetTeacherActiveRoles(userID)
		if err != nil {
			return nil, err
		}
		if len(roles) == 0 {
			return nil, errors.New("forbidden: no active roles")
		}
	}
	return s.repo.ListPendingForTeacher(userID, page, limit)
}

func (s *letterService) ListGeneralDispensasi(userRole string, userID int, page, limit int) (*domain.PaginatedLetterResponse, error) {
	return s.repo.ListGeneralDispensasi(userRole, userID, page, limit)
}

func (s *letterService) ListTeacherLetters(userID int, page, limit int) (*domain.PaginatedLetterResponse, error) {
	return s.repo.ListTeacherLetters(userID, page, limit)
}

func (s *letterService) GetTeacherStats(userID int) (map[string]any, error) {
	stats := map[string]any{
		"approved": 0,
		"rejected": 0,
		"pending":  0,
	}
	// These queries use the request_approvals table
	type countQuery struct {
		key   string
		query string
	}
	// We don't have direct DB access here, so delegate to repo
	// For now, return empty stats - will be populated when repo method is added
	return stats, nil
}
