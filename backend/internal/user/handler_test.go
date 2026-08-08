package user

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := newTestService(&mockRepository{})
	h := NewHandler(svc)
	r := gin.New()
	RegisterRoutes(r.Group("/api/v1"), h)
	return r
}

func TestCreateHandler_InvalidJSON(t *testing.T) {
	router := setupTestRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCreateHandler_MissingEmail(t *testing.T) {
	router := setupTestRouter()
	w := httptest.NewRecorder()
	body := `{"display_name": "Test User"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status code: got %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}

func TestCreateHandler_MissingDisplayName(t *testing.T) {
	router := setupTestRouter()
	w := httptest.NewRecorder()
	body := `{"email": "test@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status code: got %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}

func TestCreateHandler_InvalidEmail(t *testing.T) {
	router := setupTestRouter()
	w := httptest.NewRecorder()
	body := `{"email": "not-an-email", "display_name": "Test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status code: got %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}

func TestGetHandler_InvalidUUID(t *testing.T) {
	router := setupTestRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/not-a-uuid", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestListHandler_InvalidStatus(t *testing.T) {
	router := setupTestRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users?status=invalid", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestUpdateHandler_NoFields(t *testing.T) {
	router := setupTestRouter()
	w := httptest.NewRecorder()
	body := `{}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/"+uuid.New().String(), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestUpdateHandler_InvalidUUID(t *testing.T) {
	router := setupTestRouter()
	w := httptest.NewRecorder()
	body := `{"email": "new@example.com"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/bad-uuid", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDeactivateHandler_InvalidUUID(t *testing.T) {
	router := setupTestRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/bad-uuid/deactivate", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestReactivateHandler_InvalidUUID(t *testing.T) {
	router := setupTestRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/bad-uuid/reactivate", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}
