package httputil

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	platformerrors "github.com/conduit-platform/conduit/backend/internal/platform/errors"
)

// JSON sends a success response with the given status code and data.
func JSON(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"data": data})
}

// Error maps an error to the appropriate HTTP status and error response.
// Infrastructure errors are logged but never exposed to clients.
func Error(c *gin.Context, err error) {
	status, apiErr := platformerrors.ToAPIError(err)

	// Log internal errors with full detail.
	if status == http.StatusInternalServerError {
		logger := zerolog.Ctx(c.Request.Context())
		logger.Error().Err(err).Msg("internal error")
	}

	c.JSON(status, gin.H{"error": apiErr})
}

// NoContent sends a 204 No Content response.
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}
