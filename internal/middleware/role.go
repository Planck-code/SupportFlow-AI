package middleware

import (
	"slices"

	"github.com/gin-gonic/gin"
)

func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := CurrentUser(c)
		if !ok {
			authenticationError(c, 401)
			return
		}
		if !slices.Contains(roles, user.Role) {
			c.AbortWithStatusJSON(403, gin.H{"code": "forbidden", "message": "insufficient permissions", "request_id": c.GetString("request_id")})
			return
		}
		c.Next()
	}
}
