package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/Planck-code/SupportFlow-AI/internal/auth"
	"github.com/Planck-code/SupportFlow-AI/internal/dto"
	"github.com/Planck-code/SupportFlow-AI/internal/service"
	"github.com/gin-gonic/gin"
)

type LoginService interface {
	Login(context.Context, string, string) (string, error)
}

func LoginUser(login LoginService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request dto.LoginRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			writeError(c, 400, "invalid_input", "email and credentials are required JSON strings")
			return
		}
		token, err := login.Login(c.Request.Context(), request.Email, request.Password)
		switch {
		case errors.Is(err, service.ErrInvalidInput):
			writeError(c, 400, "invalid_input", err.Error())
		case errors.Is(err, service.ErrInvalidCredentials):
			writeError(c, 401, "invalid_credentials", "invalid email or credentials")
		case err != nil:
			writeError(c, 500, "internal_error", "internal server error")
		default:
			c.Header("Cache-Control", "no-store")
			c.JSON(http.StatusOK, dto.LoginResponse{AccessToken: token, TokenType: "Bearer", ExpiresIn: int64(auth.AccessTokenLifetime.Seconds())})
		}
	}
}
