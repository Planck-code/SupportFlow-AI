package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Planck-code/SupportFlow-AI/internal/dto"
	"github.com/Planck-code/SupportFlow-AI/internal/model"
	"github.com/Planck-code/SupportFlow-AI/internal/service"
)

type Registrar interface {
	Register(context.Context, string, string, string) (model.User, error)
}

func RegisterUser(registration Registrar) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request dto.RegisterRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			writeError(c, 400, "invalid_input", "email, password and display_name are required JSON strings")
			return
		}
		if len(request.Role) != 0 {
			writeError(c, 400, "invalid_input", "role must not be provided")
			return
		}
		user, err := registration.Register(c.Request.Context(), request.Email, request.Password, request.DisplayName)
		switch {
		case errors.Is(err, service.ErrInvalidInput):
			writeError(c, 400, "invalid_input", err.Error())
		case errors.Is(err, service.ErrEmailExists):
			writeError(c, 409, "email_exists", "email already registered")
		case err != nil:
			writeError(c, 500, "internal_error", "internal server error")
		default:
			c.JSON(http.StatusCreated, dto.NewUserResponse(user))
		}
	}
}

func writeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"code": code, "message": message, "request_id": c.GetString("request_id")})
}
