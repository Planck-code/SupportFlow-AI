package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type Pinger interface{ PingContext(context.Context) error }

func Readyz(db Pinger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"code": "database_unavailable", "message": "database is not ready",
				"request_id": c.GetString("request_id"),
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
