package middleware

import (
	"context"
	"errors"
	"strings"

	"github.com/Planck-code/SupportFlow-AI/internal/auth"
	"github.com/Planck-code/SupportFlow-AI/internal/dto"
	"github.com/Planck-code/SupportFlow-AI/internal/model"
	"github.com/Planck-code/SupportFlow-AI/internal/repository"
	"github.com/gin-gonic/gin"
)

type userByID interface {
	FindByID(context.Context, uint64) (model.User, error)
}

const currentUserKey = "authenticated_user"

// CurrentUser exposes only safe fields, never a password hash stored in Context.
func CurrentUser(c *gin.Context) (dto.UserResponse, bool) {
	value, exists := c.Get(currentUserKey)
	user, ok := value.(dto.UserResponse)
	return user, exists && ok
}

func Authenticate(tokens *auth.JWT, users userByID) gin.HandlerFunc {
	return func(c *gin.Context) {
		headers := c.Request.Header.Values("Authorization")
		if len(headers) != 1 {
			authenticationError(c, 401)
			return
		}
		parts := strings.Fields(headers[0])
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			authenticationError(c, 401)
			return
		}
		id, err := tokens.Validate(parts[1])
		if err != nil {
			authenticationError(c, 401)
			return
		}
		user, err := users.FindByID(c.Request.Context(), id)
		if errors.Is(err, repository.ErrUserNotFound) {
			authenticationError(c, 401)
			return
		}
		if err != nil {
			authenticationError(c, 500)
			return
		}
		c.Set(currentUserKey, dto.NewUserResponse(user))
		c.Next()
	}
}

func authenticationError(c *gin.Context, status int) {
	code, message := "unauthorized", "authentication required"
	if status == 500 {
		code, message = "internal_error", "internal server error"
	} else {
		c.Header("WWW-Authenticate", "Bearer")
	}
	c.AbortWithStatusJSON(status, gin.H{"code": code, "message": message, "request_id": c.GetString("request_id")})
}
