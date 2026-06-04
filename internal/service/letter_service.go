package service

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Refliqx/backend-eletter/internal/domain"
	"github.com/Refliqx/backend-eletter/internal/repository"
)

const (
	schoolStartHour = "07:00"
	schoolEndHour   = "15:00"
	schoolStart     = "07:00:00"
	schoolEnd       = "15:00:00"
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
	if req.TypeID == 0 || strings.TrimSpace(req.StartTime) == "" || strings.TrimSpace(req.EndTime) == "" {
		return 0, errors.New("type_id, start_time, dan end_time diperlukan")
	}

	// Determine the effective date: use request_date if provided, extract from start_time, or today
	effectiveDate := strings.TrimSpace(req.RequestDate)
	if effectiveDate == "" {
		effectiveDate = extractDateFromTime(req.StartTime)
	}
	if effectiveDate == "" {
		effectiveDate = time.Now().Format("2006-01-02")
	}

	// Validate date is not weekend
	if err := validateNotWeekend(effectiveDate); err != nil {
		return 0, err
	}

	// Extract and validate time is within school hours
	startTime := extractTimePart(req.StartTime)
	endTime := extractTimePart(req.EndTime)
	if startTime == "" || endTime == "" {
		return 0, errors.New("format waktu tidak valid (gunakan HH:MM:SS)")
	}
	if err := validateSchoolHours(startTime, endTime); err != nil {
		return 0, err
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

// extractTimePart extracts HH:MM:SS from either "HH:MM:SS" or "YYYY-MM-DD HH:MM:SS" format.
func extractTimePart(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) > 8 {
		// Full datetime "YYYY-MM-DD HH:MM:SS" — take just the time part after space
		if idx := strings.Index(s, " "); idx > 0 && idx+1 < len(s) {
			s = strings.TrimSpace(s[idx+1:])
		}
	}
	// Normalize HH:MM to HH:MM:SS
	if len(s) == 5 {
		s = s + ":00"
	}
	return s
}

// extractDateFromTime attempts to extract YYYY-MM-DD from a datetime string.
func extractDateFromTime(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 {
		// Check if it looks like a date prefix
		datePart := s[:10]
		if len(datePart) == 10 && datePart[4] == '-' && datePart[7] == '-' {
			return datePart
		}
	}
	return ""
}

// validateNotWeekend checks that the given YYYY-MM-DD date is not Saturday or Sunday.
func validateNotWeekend(date string) error {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return fmt.Errorf("format tanggal tidak valid: %s", date)
	}
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		return errors.New("pengajuan tidak dapat dilakukan pada akhir pekan (Sabtu/Minggu)")
	}
	return nil
}

// validateSchoolHours checks that start and end times are within school operating hours.
func validateSchoolHours(startTime, endTime string) error {
	parseTimeOnly := func(s string) (int, int, error) {
		parts := strings.Split(s, ":")
		if len(parts) < 2 {
			return 0, 0, fmt.Errorf("format waktu tidak valid: %s", s)
		}
		h, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, 0, err
		}
		m, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, err
		}
		return h, m, nil
	}

	startH, startM, err := parseTimeOnly(startTime)
	if err != nil {
		return err
	}
	endH, endM, err := parseTimeOnly(endTime)
	if err != nil {
		return err
	}

	startTotal := startH*60 + startM
	endTotal := endH*60 + endM

	// School hours: 07:00 (420 min) to 15:00 (900 min)
	if startTotal < 420 || startTotal > 900 {
		return fmt.Errorf("jam mulai (%02d:%02d) di luar jam operasional sekolah (07:00 - 15:00 WIB)", startH, startM)
	}
	if endTotal < 420 || endTotal > 900 {
		return fmt.Errorf("jam selesai (%02d:%02d) di luar jam operasional sekolah (07:00 - 15:00 WIB)", endH, endM)
	}
	if startTotal > endTotal {
		return errors.New("jam selesai harus setelah atau sama dengan jam mulai")
	}

	return nil
}
