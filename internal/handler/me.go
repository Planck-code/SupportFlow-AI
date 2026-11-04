package handler

import (
	"github.com/Planck-code/SupportFlow-AI/internal/middleware"
	"github.com/gin-gonic/gin"
	"net/http"
)

func Me(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		writeError(c, 401, "unauthorized", "authentication required")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, user)
}
