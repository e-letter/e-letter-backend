package handler

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Refliqx/backend-eletter/internal/domain"
	"github.com/Refliqx/backend-eletter/internal/response"
	"github.com/Refliqx/backend-eletter/internal/service"
	"github.com/Refliqx/backend-eletter/internal/utils"
	"github.com/gin-gonic/gin"
)

type UserProfileHandler struct {
	service service.UserProfileService
	baseURL string
	db      *sql.DB
}

func NewUserProfileHandler(s service.UserProfileService, baseURL string, db *sql.DB) *UserProfileHandler {
	return &UserProfileHandler{service: s, baseURL: baseURL, db: db}
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
	response.Success(c, http.StatusOK, "", user)
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
	utils.LogActivity(h.db, int64(userID), "update_profile", "Pembaruan profil user ID #"+strconv.Itoa(userID), c.ClientIP(), c.Request.UserAgent())
	response.Success(c, http.StatusOK, "", user)
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

	if req.SignatureDataUrl == "" || !strings.HasPrefix(req.SignatureDataUrl, "data:image/png;base64,") {
		response.Error(c, http.StatusBadRequest, "Format tanda tangan harus PNG base64 (dimulai dengan data:image/png;base64,)")
		return
	}
	if req.Role == "" {
		response.Error(c, http.StatusBadRequest, "Role tidak boleh kosong")
		return
	}

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

	pngData := req.SignatureDataUrl
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(pngData, "data:image/png;base64,"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Gagal mendecode tanda tangan")
		return
	}

	signaturesDir := "./public/uploads/signatures"
	if err := os.MkdirAll(signaturesDir, 0755); err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal membuat direktori tanda tangan")
		return
	}

	absSignaturesDir, err := filepath.Abs(signaturesDir)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal memproses path")
		return
	}

	filename := fmt.Sprintf("%s_%d_ttd.png", req.Role, userID)
	filePath := filepath.Join(signaturesDir, filename)

	absFilePath, err := filepath.Abs(filePath)
	if err != nil || !strings.HasPrefix(absFilePath, absSignaturesDir) {
		response.Error(c, http.StatusBadRequest, "Path file tidak valid")
		return
	}

	if err := os.WriteFile(absFilePath, decoded, 0644); err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal menyimpan tanda tangan")
		return
	}

	signatureURL := strings.TrimRight(h.baseURL, "/") + "/signatures/" + filename
	utils.LogActivity(h.db, int64(userID), "upload_signature", "Unggah tanda tangan user ID #"+strconv.Itoa(userID), c.ClientIP(), c.Request.UserAgent())
	response.Success(c, http.StatusOK, "Tanda tangan berhasil disimpan", gin.H{"signature_url": signatureURL})
}

func (h *UserProfileHandler) CompleteOnboarding(c *gin.Context) {
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

		utils.LogActivity(h.db, int64(userID), "complete_onboarding", "Onboarding guru selesai user ID #"+strconv.Itoa(userID), c.ClientIP(), c.Request.UserAgent())
		response.Success(c, http.StatusOK, "Onboarding guru berhasil diselesaikan", gin.H{
			"userId":           updatedUser.ID,
			"profileCompleted": updatedUser.ProfileCompleted,
		})
		return
	}

	var req struct {
		FullName          *string `json:"fullName"`
		FullNameSnake     *string `json:"full_name"`
		NIP               *string `json:"nip"`
		Gender            *string `json:"gender"`
		SignatureUrl      *string `json:"signatureUrl"`
		SignatureUrlSnake *string `json:"signature_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	fullName := req.FullName
	if fullName == nil {
		fullName = req.FullNameSnake
	}
	signatureUrl := req.SignatureUrl
	if signatureUrl == nil {
		signatureUrl = req.SignatureUrlSnake
	}

	updatedUser, err := h.service.UpdateProfile(domain.UserProfileUpdatePayload{
		UserID: userID,
		UserProfileUpdateRequest: domain.UserProfileUpdateRequest{
			FullName:          fullName,
			NIP:               req.NIP,
			Gender:            req.Gender,
			SignatureUrl:      signatureUrl,
			MarkProfileFinish: true,
		},
	})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal menyelesaikan onboarding: "+err.Error())
		return
	}

	utils.LogActivity(h.db, int64(userID), "complete_onboarding", "Onboarding selesai user ID #"+strconv.Itoa(userID), c.ClientIP(), c.Request.UserAgent())
	response.Success(c, http.StatusOK, "Onboarding berhasil diselesaikan", gin.H{
		"userId":           updatedUser.ID,
		"profileCompleted": updatedUser.ProfileCompleted,
	})
}

func (h *UserProfileHandler) GetSchedules(c *gin.Context) {
	userIDVal, ok := c.Get("userId")
	if !ok {
		response.Error(c, http.StatusUnauthorized, "Token akses diperlukan")
		return
	}
	userID, _ := userIDVal.(int)

	schedules, err := h.service.GetSchedules(userID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal mengambil jadwal: "+err.Error())
		return
	}

	response.Success(c, http.StatusOK, "", schedules)
}
