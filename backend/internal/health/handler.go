package health

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Handler provides health check endpoints.
type Handler struct {
	db    *pgxpool.Pool
	redis *redis.Client
}

// NewHandler creates a new health check handler.
func NewHandler(db *pgxpool.Pool, redis *redis.Client) *Handler {
	return &Handler{db: db, redis: redis}
}

type componentStatus struct {
	Status string `json:"status"`
}

type readyResponse struct {
	Status   string          `json:"status"`
	Database componentStatus `json:"database"`
	Redis    componentStatus `json:"redis"`
}

// Health returns basic application health. If the process is running, it's healthy.
// GET /health
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"service": "conduit",
	})
}

// Ready checks all dependencies for readiness.
// GET /ready
func (h *Handler) Ready(c *gin.Context) {
	ctx := c.Request.Context()
	overallStatus := http.StatusOK

	resp := readyResponse{
		Status:   "ready",
		Database: componentStatus{Status: "up"},
		Redis:    componentStatus{Status: "up"},
	}

	// Check database.
	if err := h.db.Ping(ctx); err != nil {
		resp.Database.Status = "down"
		resp.Status = "not_ready"
		overallStatus = http.StatusServiceUnavailable
	}

	// Check Redis.
	if err := h.redis.Ping(ctx).Err(); err != nil {
		resp.Redis.Status = "down"
		resp.Status = "not_ready"
		overallStatus = http.StatusServiceUnavailable
	}

	c.JSON(overallStatus, resp)
}

// Live indicates whether the application is alive (not deadlocked).
// GET /live
func (h *Handler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "alive",
	})
}
