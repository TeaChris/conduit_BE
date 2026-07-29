package user

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/conduit-platform/conduit/backend/internal/platform/errors"
	"github.com/conduit-platform/conduit/backend/pkg/httputil"
)

// Handler handles HTTP requests for the user domain.
type Handler struct {
	service *Service
}

// NewHandler creates a new user Handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes registers user routes on the given router group.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler) {
	users := rg.Group("/users")
	{
		users.POST("", h.Create)
		users.GET("", h.List)
		users.GET("/:id", h.Get)
		users.PATCH("/:id", h.Update)
		users.POST("/:id/deactivate", h.Deactivate)
		users.POST("/:id/reactivate", h.Reactivate)
	}
}

// Create handles POST /api/v1/users.
func (h *Handler) Create(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.Error(c, errors.NewBadRequest("invalid request body"))
		return
	}

	if fieldErrors := validateCreateRequest(req); len(fieldErrors) > 0 {
		httputil.Error(c, errors.NewValidationError("invalid input", fieldErrors...))
		return
	}

	input := CreateUserInput{
		Email:       req.Email,
		DisplayName: req.DisplayName,
		Metadata:    req.Metadata,
	}

	user, err := h.service.CreateUser(c.Request.Context(), input)
	if err != nil {
		httputil.Error(c, err)
		return
	}

	httputil.JSON(c, http.StatusCreated, toResponse(user))
}

// Get handles GET /api/v1/users/:id.
func (h *Handler) Get(c *gin.Context) {
	userID, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	user, err := h.service.GetUser(c.Request.Context(), userID)
	if err != nil {
		httputil.Error(c, err)
		return
	}

	httputil.JSON(c, http.StatusOK, toResponse(user))
}

// List handles GET /api/v1/users.
func (h *Handler) List(c *gin.Context) {
	filter := ListFilter{
		Page:     parseIntQuery(c, "page", 1),
		PageSize: parseIntQuery(c, "per_page", 20),
	}

	if statusStr := c.Query("status"); statusStr != "" {
		status := Status(statusStr)
		if !status.IsValid() {
			httputil.Error(c, errors.NewBadRequest("invalid status filter"))
			return
		}
		filter.Status = &status
	}

	users, total, err := h.service.ListUsers(c.Request.Context(), filter)
	if err != nil {
		httputil.Error(c, err)
		return
	}

	httputil.JSON(c, http.StatusOK, UserListResponse{
		Users:      toResponseList(users),
		Pagination: newPagination(filter.Page, filter.PageSize, total),
	})
}

// Update handles PATCH /api/v1/users/:id.
func (h *Handler) Update(c *gin.Context) {
	userID, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.Error(c, errors.NewBadRequest("invalid request body"))
		return
	}

	if req.Email == nil && req.DisplayName == nil && req.Metadata == nil {
		httputil.Error(c, errors.NewBadRequest("at least one field must be provided"))
		return
	}

	if fieldErrors := validateUpdateRequest(req); len(fieldErrors) > 0 {
		httputil.Error(c, errors.NewValidationError("invalid input", fieldErrors...))
		return
	}

	input := UpdateUserInput{
		Email:       req.Email,
		DisplayName: req.DisplayName,
		Metadata:    req.Metadata,
	}

	user, err := h.service.UpdateUser(c.Request.Context(), userID, input)
	if err != nil {
		httputil.Error(c, err)
		return
	}

	httputil.JSON(c, http.StatusOK, toResponse(user))
}

// Deactivate handles POST /api/v1/users/:id/deactivate.
func (h *Handler) Deactivate(c *gin.Context) {
	userID, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	user, err := h.service.DeactivateUser(c.Request.Context(), userID)
	if err != nil {
		httputil.Error(c, err)
		return
	}

	httputil.JSON(c, http.StatusOK, toResponse(user))
}

// Reactivate handles POST /api/v1/users/:id/reactivate.
func (h *Handler) Reactivate(c *gin.Context) {
	userID, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	user, err := h.service.ReactivateUser(c.Request.Context(), userID)
	if err != nil {
		httputil.Error(c, err)
		return
	}

	httputil.JSON(c, http.StatusOK, toResponse(user))
}

// --- Validation ---

func validateCreateRequest(req CreateUserRequest) []errors.FieldError {
	var errs []errors.FieldError

	email := strings.TrimSpace(req.Email)
	if email == "" {
		errs = append(errs, errors.FieldError{Field: "email", Message: "is required"})
	} else if !strings.Contains(email, "@") {
		errs = append(errs, errors.FieldError{Field: "email", Message: "must be a valid email address"})
	} else if len(email) > 320 {
		errs = append(errs, errors.FieldError{Field: "email", Message: "must be at most 320 characters"})
	}

	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		errs = append(errs, errors.FieldError{Field: "display_name", Message: "is required"})
	} else if len(displayName) > 256 {
		errs = append(errs, errors.FieldError{Field: "display_name", Message: "must be at most 256 characters"})
	}

	return errs
}

func validateUpdateRequest(req UpdateUserRequest) []errors.FieldError {
	var errs []errors.FieldError

	if req.Email != nil {
		email := strings.TrimSpace(*req.Email)
		if email == "" {
			errs = append(errs, errors.FieldError{Field: "email", Message: "cannot be empty"})
		} else if !strings.Contains(email, "@") {
			errs = append(errs, errors.FieldError{Field: "email", Message: "must be a valid email address"})
		} else if len(email) > 320 {
			errs = append(errs, errors.FieldError{Field: "email", Message: "must be at most 320 characters"})
		}
	}

	if req.DisplayName != nil {
		displayName := strings.TrimSpace(*req.DisplayName)
		if displayName == "" {
			errs = append(errs, errors.FieldError{Field: "display_name", Message: "cannot be empty"})
		} else if len(displayName) > 256 {
			errs = append(errs, errors.FieldError{Field: "display_name", Message: "must be at most 256 characters"})
		}
	}

	return errs
}

// --- Helpers ---

// parseUUID extracts and validates a UUID path parameter.
func parseUUID(c *gin.Context, param string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Param(param))
	if err != nil {
		httputil.Error(c, errors.NewBadRequest("invalid "+param+" format: must be a valid UUID"))
		return uuid.Nil, err
	}
	return id, nil
}

// parseIntQuery extracts an integer query parameter with a default value.
func parseIntQuery(c *gin.Context, key string, defaultVal int) int {
	val := c.Query(key)
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil || n < 1 {
		return defaultVal
	}
	return n
}
