package handler

import (
	"net/http"
	"strconv"

	"github.com/Refliqx/backend-eletter/internal/response"
	"github.com/Refliqx/backend-eletter/internal/service"
	"github.com/gin-gonic/gin"
)

type MasterDataHandler struct {
	service service.MasterDataService
}

func NewMasterDataHandler(s service.MasterDataService) *MasterDataHandler {
	return &MasterDataHandler{service: s}
}

func (h *MasterDataHandler) GetClasses(c *gin.Context) {
	classes, err := h.service.GetAllClasses(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal mengambil data kelas")
		return
	}
	response.Success(c, http.StatusOK, "Berhasil mengambil data kelas", classes)
}

func (h *MasterDataHandler) GetClass(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "ID kelas tidak valid")
		return
	}

	class, err := h.service.GetClassByID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal mengambil data kelas")
		return
	}
	if class == nil {
		response.Error(c, http.StatusNotFound, "Kelas tidak ditemukan")
		return
	}
	response.Success(c, http.StatusOK, "Berhasil mengambil data kelas", class)
}

func (h *MasterDataHandler) GetMajors(c *gin.Context) {
	majors, err := h.service.GetAllMajors(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal mengambil data konsentrasi keahlian")
		return
	}
	response.Success(c, http.StatusOK, "Berhasil mengambil data konsentrasi keahlian", majors)
}

func (h *MasterDataHandler) GetMajor(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "ID konsentrasi keahlian tidak valid")
		return
	}

	major, err := h.service.GetMajorByID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal mengambil data konsentrasi keahlian")
		return
	}
	if major == nil {
		response.Error(c, http.StatusNotFound, "Konsentrasi Keahlian tidak ditemukan")
		return
	}
	response.Success(c, http.StatusOK, "Berhasil mengambil data konsentrasi keahlian", major)
}

func (h *MasterDataHandler) GetStudents(c *gin.Context) {
	var classID, majorID *int64

	if classIDStr := c.Query("class_id"); classIDStr != "" {
		id, err := strconv.ParseInt(classIDStr, 10, 64)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "ID kelas tidak valid")
			return
		}
		classID = &id
	}

	if majorIDStr := c.Query("major_id"); majorIDStr != "" {
		id, err := strconv.ParseInt(majorIDStr, 10, 64)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "ID konsentrasi keahlian tidak valid")
			return
		}
		majorID = &id
	}

	students, err := h.service.GetStudents(c.Request.Context(), classID, majorID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Gagal mengambil data siswa")
		return
	}
	response.Success(c, http.StatusOK, "Berhasil mengambil data siswa", students)
}
