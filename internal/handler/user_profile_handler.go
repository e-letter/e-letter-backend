package handler

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Refliqx/backend-eletter/internal/domain"
	"github.com/Refliqx/backend-eletter/internal/response"
	"github.com/Refliqx/backend-eletter/internal/service"
	"github.com/gin-gonic/gin"
)

type UserProfileHandler struct {
	service service.UserProfileService
}

func NewUserProfileHandler(s service.UserProfileService) *UserProfileHandler {
	return &UserProfileHandler{service: s}
}

func (h *UserProfileHandler) GetProfile(c *gin.Context) {
	userIDVal, ok := c.Get("userId")
	if !ok {
		response.Error(c, http.StatusUnauthorized, "Token akses diperlukan")
		return
	}
	userID, _ := userIDVal.(int)
	user, err := h.service.GetProfile(userID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal mengambil data profil: "+err.Error())
		return
	}
	if user == nil {
		response.Error(c, http.StatusNotFound, "User tidak ditemukan")
		return
	}
	response.Raw(c, http.StatusOK, gin.H{
		"success": true,
		"data":    user,
	})
}

func (h *UserProfileHandler) UpdateProfile(c *gin.Context) {
	userIDVal, ok := c.Get("userId")
	if !ok {
		response.Error(c, http.StatusUnauthorized, "Token akses diperlukan")
		return
	}
	userID, _ := userIDVal.(int)

	var req domain.UserProfileUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	payload := domain.UserProfileUpdatePayload{
		UserID:                   userID,
		UserProfileUpdateRequest: req,
	}

	user, err := h.service.UpdateProfile(payload)
	if err != nil {
		if err.Error() == "Missing userId" {
			response.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": user})
}

func (h *UserProfileHandler) UploadSignature(c *gin.Context) {
	userIDVal, ok := c.Get("userId")
	if !ok {
		response.Error(c, http.StatusUnauthorized, "Token akses diperlukan")
		return
	}
	userID, _ := userIDVal.(int)

	var req struct {
		SignatureDataUrl string `json:"signatureDataUrl"`
		Role             string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	if req.SignatureDataUrl == "" || !strings.HasPrefix(req.SignatureDataUrl, "<svg") {
		response.Error(c, http.StatusBadRequest, "Format tanda tangan harus SVG (dimulai dengan <svg)")
		return
	}
	if req.Role == "" {
		response.Error(c, http.StatusBadRequest, "Role tidak boleh kosong")
		return
	}

	// Validate role against allowlist to prevent path traversal
	allowedRoles := map[string]bool{
		"student":        true,
		"teacher":        true,
		"admin":          true,
		"kepala_sekolah": true,
	}
	if !allowedRoles[req.Role] {
		response.Error(c, http.StatusBadRequest, "Role tidak valid")
		return
	}

	svgData := req.SignatureDataUrl
	if strings.HasPrefix(svgData, "data:image/svg+xml;base64,") {
		decoded, err := base64.StdEncoding.DecodeString(svgData[26:])
		if err != nil {
			response.Error(c, http.StatusBadRequest, "Gagal mendecode tanda tangan")
			return
		}
		svgData = string(decoded)
	}

	signaturesDir := "./public/uploads/signatures"
	if err := os.MkdirAll(signaturesDir, 0755); err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal membuat direktori tanda tangan")
		return
	}

	// Verify resolved path stays within the signatures directory
	absSignaturesDir, err := filepath.Abs(signaturesDir)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal memproses path")
		return
	}

	filename := fmt.Sprintf("%s_%d_ttd.svg", req.Role, userID)
	filePath := filepath.Join(signaturesDir, filename)

	absFilePath, err := filepath.Abs(filePath)
	if err != nil || !strings.HasPrefix(absFilePath, absSignaturesDir) {
		response.Error(c, http.StatusBadRequest, "Path file tidak valid")
		return
	}

	if err := os.WriteFile(absFilePath, []byte(svgData), 0644); err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal menyimpan tanda tangan")
		return
	}

	// Store signature URL using the storage domain.
	signatureURL := "https://storage.smkn2singosari.sch.id/signatures/" + filename
	response.Success(c, http.StatusOK, "Tanda tangan berhasil disimpan", gin.H{"signature_url": signatureURL})
}

func (h *UserProfileHandler) CompleteOnboarding(c *gin.Context) {
	userIDVal, ok := c.Get("userId")
	if !ok {
		response.Error(c, http.StatusUnauthorized, "Token akses diperlukan")
		return
	}
	userID, _ := userIDVal.(int)

	// Fetch current user profile to determine their role
	user, err := h.service.GetProfile(userID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal mengambil data profil: "+err.Error())
		return
	}
	if user == nil {
		response.Error(c, http.StatusNotFound, "User tidak ditemukan")
		return
	}

	if user.Role == "teacher" {
		var req domain.CompleteTeacherOnboardingPayload
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		req.UserID = userID

		updatedUser, err := h.service.CompleteTeacherOnboarding(req)
		if err != nil {
			response.Error(c, http.StatusInternalServerError, "Gagal menyelesaikan onboarding guru: "+err.Error())
			return
		}

		response.Success(c, http.StatusOK, "Onboarding guru berhasil diselesaikan", gin.H{
			"userId":           updatedUser.ID,
			"profileCompleted": updatedUser.ProfileCompleted,
		})
		return
	}

	// For student and others
	var req struct {
		FullName     *string `json:"fullName"`
		NIP          *string `json:"nip"`
		Gender       *string `json:"gender"`
		SignatureUrl *string `json:"signatureUrl"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	updatedUser, err := h.service.UpdateProfile(domain.UserProfileUpdatePayload{
		UserID: userID,
		UserProfileUpdateRequest: domain.UserProfileUpdateRequest{
			FullName:          req.FullName,
			NIP:               req.NIP,
			Gender:            req.Gender,
			SignatureUrl:      req.SignatureUrl,
			MarkProfileFinish: true,
		},
	})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal menyelesaikan onboarding: "+err.Error())
		return
	}

	response.Success(c, http.StatusOK, "Onboarding berhasil diselesaikan", gin.H{
		"userId":           updatedUser.ID,
		"profileCompleted": updatedUser.ProfileCompleted,
	})
}
