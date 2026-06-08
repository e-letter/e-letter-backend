package handler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Refliqx/backend-eletter/internal/domain"
	"github.com/Refliqx/backend-eletter/internal/handler"
	"github.com/Refliqx/backend-eletter/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPermissionService struct {
	approveFunc func(req domain.ApprovalRequest, approverID int) error
	deleteFunc  func(id int) error
}

var _ service.PermissionService = (*mockPermissionService)(nil)

func (m *mockPermissionService) Get(_, _, _ string, _ int, _ int, _, _ string) (any, error) {
	return nil, nil
}
func (m *mockPermissionService) Create(_ domain.CreatePermissionRequest) (int, error) { return 0, nil }
func (m *mockPermissionService) Update(_ domain.UpdatePermissionRequest) error        { return nil }
func (m *mockPermissionService) Delete(id int) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(id)
	}
	return nil
}
func (m *mockPermissionService) Approve(req domain.ApprovalRequest, approverID int) error {
	if m.approveFunc != nil {
		return m.approveFunc(req, approverID)
	}
	return nil
}
func (m *mockPermissionService) ListRegistrationTokens() ([]domain.TokenRecord, error) {
	return nil, nil
}
func (m *mockPermissionService) UpsertRegistrationToken(_ string, _ int, _ *int, _ *time.Time) (*domain.TokenRecord, error) {
	return nil, nil
}
func (m *mockPermissionService) CancelRequest(_, _ int, _ string) error { return nil }
func (m *mockPermissionService) GetRequestDetail(_ int) (any, error)    { return nil, nil }
func (m *mockPermissionService) GetTeacherRoles(_ int) (any, error)     { return nil, nil }
func (m *mockPermissionService) RequestTeacherRole(_ int, _ string, _ domain.TeacherRoleMetadata) error {
	return nil
}
func (m *mockPermissionService) CreateDelegation(_, _ int, _, _, _ string) error { return nil }
func (m *mockPermissionService) ListDelegations(_ int) (any, error)              { return nil, nil }
func (m *mockPermissionService) DeleteDelegation(_, _ int) error                 { return nil }

func setupRouter(svc service.PermissionService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	h := handler.NewPermissionHandler(svc, false, nil)

	authMiddleware := func(c *gin.Context) {
		c.Set("userId", 1)
		c.Set("userRole", "teacher")
		c.Next()
	}

	r.POST("/approve", authMiddleware, h.Approve)
	r.DELETE("/requests", authMiddleware, h.DeleteRequest)
	r.POST("/requests", authMiddleware, h.CreateRequest)

	return r
}

func doRequest(t *testing.T, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestApprove_Success(t *testing.T) {
	svc := &mockPermissionService{
		approveFunc: func(req domain.ApprovalRequest, approverID int) error {
			assert.Equal(t, 42, req.RequestID)
			assert.Equal(t, 2, req.StageID)
			assert.Equal(t, "approved", req.Status)
			assert.Equal(t, 1, approverID)
			return nil
		},
	}

	router := setupRouter(svc)
	payload := map[string]any{
		"request_id": 42,
		"stage_id":   2,
		"status":     "approved",
		"notes":      nil,
	}

	w := doRequest(t, router, http.MethodPost, "/approve", payload)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["success"])
}

func TestApprove_ServiceError(t *testing.T) {
	svc := &mockPermissionService{
		approveFunc: func(_ domain.ApprovalRequest, _ int) error {
			return errors.New("db connection lost")
		},
	}

	router := setupRouter(svc)
	payload := map[string]any{
		"request_id": 42,
		"stage_id":   2,
		"status":     "approved",
	}

	w := doRequest(t, router, http.MethodPost, "/approve", payload)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestApprove_InvalidStatus(t *testing.T) {
	svc := &mockPermissionService{
		approveFunc: func(_ domain.ApprovalRequest, _ int) error {
			return errors.New("invalid status: must be approved, rejected, or skipped")
		},
	}

	router := setupRouter(svc)
	payload := map[string]any{
		"request_id": 42,
		"stage_id":   2,
		"status":     "banana",
	}

	w := doRequest(t, router, http.MethodPost, "/approve", payload)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestApprove_MissingBody(t *testing.T) {
	svc := &mockPermissionService{}
	router := setupRouter(svc)

	w := doRequest(t, router, http.MethodPost, "/approve", nil)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDeleteRequest_MissingID(t *testing.T) {
	svc := &mockPermissionService{
		deleteFunc: func(id int) error {
			if id == 0 {
				return errors.New("id is required")
			}
			return nil
		},
	}
	router := setupRouter(svc)

	req := httptest.NewRequest(http.MethodDelete, "/requests", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
