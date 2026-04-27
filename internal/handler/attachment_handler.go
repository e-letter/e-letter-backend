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

	filePath := filepath.Join("public", strings.TrimPrefix(attachment.FileURL, "/"))
	buf, readErr := os.ReadFile(filePath)
	if readErr != nil {
		response.Raw(c, http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	contentType := "application/octet-stream"
	if attachment.MimeType != nil && *attachment.MimeType != "" {
		contentType = *attachment.MimeType
	}
	c.Header("Content-Type", contentType)
	if attachment.FileName != nil {
		c.Header("Content-Disposition", `attachment; filename="`+*attachment.FileName+`"`)
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
