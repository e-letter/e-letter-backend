package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Refliqx/backend-eletter/internal/response"
	"github.com/Refliqx/backend-eletter/internal/service"
	"github.com/gin-gonic/gin"
)

type AttachmentHandler struct {
	service service.AttachmentService
}

func NewAttachmentHandler(s service.AttachmentService) *AttachmentHandler {
	return &AttachmentHandler{service: s}
}

func (h *AttachmentHandler) GetByID(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	attachment, err := h.service.GetByID(id)
	if err != nil {
		if err.Error() == "Invalid attachment ID" {
			response.Raw(c, http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		response.Raw(c, http.StatusNotFound, gin.H{"error": "Attachment not found"})
		return
	}

	userID := toIntFromContext(c, "userId")
	if userID <= 0 {
		response.Raw(c, http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	relPath := strings.TrimPrefix(attachment.FilePath, "/")
	filePath := filepath.Join("public", relPath)
	cleanPath := filepath.Clean(filePath)
	baseDir := filepath.Clean("public")
	if !strings.HasPrefix(cleanPath, baseDir+string(filepath.Separator)) && cleanPath != baseDir {
		response.Raw(c, http.StatusForbidden, gin.H{"error": "Invalid file path"})
		return
	}

	buf, readErr := os.ReadFile(cleanPath)
	if readErr != nil {
		response.Raw(c, http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	contentType := "application/octet-stream"
	if attachment.MimeType != nil && *attachment.MimeType != "" {
		contentType = *attachment.MimeType
	}
	c.Header("Content-Type", contentType)
	if attachment.OriginalName != "" {
		c.Header("Content-Disposition", `attachment; filename="`+attachment.OriginalName+`"`)
	}
	c.Data(http.StatusOK, contentType, buf)
}

func (h *AttachmentHandler) ListByRequest(c *gin.Context) {
	requestID, _ := strconv.Atoi(c.Param("requestId"))
	items, err := h.service.GetByRequestID(requestID)
	if err != nil {
		if err.Error() == "Invalid request ID" {
			response.Raw(c, http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		response.Raw(c, http.StatusInternalServerError, gin.H{"success": false, "error": "Error retrieving attachments: " + err.Error()})
		return
	}
	response.Raw(c, http.StatusOK, gin.H{"success": true, "data": items})
}

func (h *AttachmentHandler) Upload(c *gin.Context) {
	requestID, _ := strconv.Atoi(c.PostForm("request_id"))
	if requestID == 0 {
		response.Error(c, http.StatusBadRequest, "request_id diperlukan")
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "File diperlukan")
		return
	}

	// Limit 10MB
	if file.Size > 10*1024*1024 {
		response.Error(c, http.StatusBadRequest, "Ukuran file maksimal 10MB")
		return
	}

	uploadDir := "public/uploads/attachments"
	os.MkdirAll(uploadDir, 0755)

	ext := filepath.Ext(file.Filename)
	filename := strconv.FormatInt(int64(requestID), 10) + "_" + strconv.FormatInt(int64(os.Getpid()), 10) + ext
	dst := filepath.Join(uploadDir, filename)

	if err := c.SaveUploadedFile(file, dst); err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal menyimpan file")
		return
	}

	filePath := "/uploads/attachments/" + filename
	mimeType := file.Header.Get("Content-Type")
	id, err := h.service.Create(requestID, filePath, file.Filename, mimeType, file.Size)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal menyimpan data attachment")
		return
	}

	response.Raw(c, http.StatusCreated, gin.H{"success": true, "data": gin.H{"id": id, "file_path": filePath}})
}
