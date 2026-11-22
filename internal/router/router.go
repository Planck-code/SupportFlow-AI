package router

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Planck-code/SupportFlow-AI/internal/handler"
	"github.com/Planck-code/SupportFlow-AI/internal/middleware"
	"github.com/Planck-code/SupportFlow-AI/internal/model"
)

func New(logger *slog.Logger, db handler.Pinger, registration handler.Registrar, login handler.LoginService, authenticate gin.HandlerFunc, agents handler.AgentDirectory, tickets handler.Tickets) *gin.Engine {
	r := gin.New()
	r.Use(middleware.RequestLog(logger))
	// Gin's default recovery dumps request headers. Keep panic logs metadata-only.
	r.Use(gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) {
		logger.Error("request panic", "request_id", c.GetString("request_id"))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"code":       "internal_error",
			"message":    "internal server error",
			"request_id": c.GetString("request_id"),
		})
	}))
	r.GET("/healthz", handler.Healthz)
	r.GET("/readyz", handler.Readyz(db))
	r.POST("/api/v1/auth/register", handler.RegisterUser(registration))
	r.POST("/api/v1/auth/login", handler.LoginUser(login))
	r.GET("/api/v1/users/me", authenticate, handler.Me)
	r.GET("/api/v1/admin/agents", authenticate, middleware.RequireRole(model.RoleAdmin), handler.ListAgents(agents))
	group := r.Group("/api/v1/tickets", authenticate)
	group.POST("", middleware.RequireRole(model.RoleUser), tickets.Create)
	group.GET("", tickets.List)
	group.GET("/queue", middleware.RequireRole(model.RoleAgent), tickets.Queue)
	group.GET("/:id", tickets.Detail)
	group.GET("/:id/events", tickets.Events)
	return r
}
