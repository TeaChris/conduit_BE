package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/conduit-platform/conduit/backend/internal/platform/tenant"
)

const (
	// TenantIDHeader is the header key for tenant identification.
	TenantIDHeader = "X-Tenant-ID"
)

// TenantID extracts the tenant ID from the request header and stores it in context.
// Returns 400 Bad Request if the header is missing.
func TenantID() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.GetHeader(TenantIDHeader)
		if tenantID == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": gin.H{
					"code":    "TENANT_REQUIRED",
					"message": "X-Tenant-ID header is required.",
				},
			})
			return
		}

		ctx := tenant.SetID(c.Request.Context(), tenantID)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}
