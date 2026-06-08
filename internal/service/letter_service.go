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
	if req.TypeID == 0 {
		return 0, errors.New("type_id diperlukan")
	}

	userRole, err := s.repo.GetUserRole(userID)
	if err != nil {
		return 0, errors.New("User pengaju tidak ditemukan")
	}

	typeInfo, err := s.repo.GetRequestTypeInfo(req.TypeID)
	if err != nil {
		return 0, errors.New("Jenis surat tidak ditemukan")
	}
	if !typeInfo.IsActive {
		return 0, errors.New("Jenis surat tidak aktif")
	}

	if typeInfo.RequesterRole == "student" && userRole != "student" {
		return 0, errors.New("Izin keluar/masuk hanya boleh diajukan oleh siswa")
	}
	if typeInfo.RequesterRole == "teacher" && userRole != "teacher" && userRole != "kepala_sekolah" {
		return 0, errors.New("Dispensasi hanya boleh diajukan oleh guru")
	}

	if strings.TrimSpace(req.StartTime) == "" {
		return 0, errors.New("start_time diperlukan")
	}
	if strings.TrimSpace(req.EndTime) == "" {
		req.EndTime = req.StartTime
	}

	effectiveDate := strings.TrimSpace(req.RequestDate)
	if effectiveDate == "" {
		effectiveDate = extractDateFromTime(req.StartTime)
	}
	if effectiveDate == "" {
		effectiveDate = time.Now().Format("2006-01-02")
	}

	if userRole == "student" {
		hasActive, err := s.repo.HasActiveRequest(userID, req.TypeID, effectiveDate)
		if err != nil {
			return 0, err
		}
		if hasActive {
			return 0, errors.New("Sudah ada surat izin aktif untuk tanggal dan tipe yang sama")
		}
	}

	if err := validateNotWeekend(effectiveDate); err != nil {
		return 0, err
	}

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

		filename := fmt.Sprintf("student_%d_%d.png", userID, time.Now().UnixNano())
		filePath := filepath.Join("public", "uploads", "signatures", filename)

		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return 0, fmt.Errorf("failed to create signature directory: %w", err)
		}

		if err := os.WriteFile(filePath, decoded, 0644); err != nil {
			return 0, fmt.Errorf("failed to save signature file: %w", err)
		}

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
	type countQuery struct {
		key   string
		query string
	}
	return stats, nil
}

func extractTimePart(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) > 8 {
		if idx := strings.Index(s, " "); idx > 0 && idx+1 < len(s) {
			s = strings.TrimSpace(s[idx+1:])
		}
	}
	if len(s) == 5 {
		s = s + ":00"
	}
	return s
}

func extractDateFromTime(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 {
		datePart := s[:10]
		if len(datePart) == 10 && datePart[4] == '-' && datePart[7] == '-' {
			return datePart
		}
	}
	return ""
}

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

	startTotal := startH*60 + startM

	if startTotal < 420 || startTotal > 900 {
		return fmt.Errorf("jam mulai (%02d:%02d) di luar jam operasional sekolah (07:00 - 15:00 WIB)", startH, startM)
	}

	endH, endM, err := parseTimeOnly(endTime)
	if err != nil {
		return err
	}
	endTotal := endH*60 + endM

	if endTotal < 420 || endTotal > 900 {
		return fmt.Errorf("jam selesai (%02d:%02d) di luar jam operasional sekolah (07:00 - 15:00 WIB)", endH, endM)
	}
	if startTotal > endTotal {
		return errors.New("jam selesai harus setelah atau sama dengan jam mulai")
	}

	return nil
}
