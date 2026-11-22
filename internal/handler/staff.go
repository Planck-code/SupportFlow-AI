package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Planck-code/SupportFlow-AI/internal/dto"
	"github.com/Planck-code/SupportFlow-AI/internal/middleware"
	"github.com/Planck-code/SupportFlow-AI/internal/service"
)

// Decode exactly one object, with each of the four case-sensitive keys present once.
func decodeStaff(body io.Reader) (dto.CreateStaffRequest, error) {
	var in dto.CreateStaffRequest
	fields := map[string]*string{"email": &in.Email, "password": &in.Password, "display_name": &in.DisplayName, "role": &in.Role}
	d := json.NewDecoder(body)
	open, err := d.Token()
	if err != nil || open != json.Delim('{') {
		return in, service.ErrInvalidInput
	}
	for d.More() {
		key, err := d.Token()
		name, ok := key.(string)
		dest := fields[name]
		if err != nil || !ok || dest == nil {
			return in, service.ErrInvalidInput
		}
		if err := d.Decode(dest); err != nil || *dest == "" {
			return in, service.ErrInvalidInput
		}
		delete(fields, name)
	}
	end, err := d.Token()
	if err != nil || end != json.Delim('}') || len(fields) != 0 {
		return in, service.ErrInvalidInput
	}
	if _, err := d.Token(); err != io.EOF {
		return in, service.ErrInvalidInput
	}
	return in, nil
}

func CreateStaff(reg *service.Registration, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := middleware.CurrentUser(c)
		if !ok {
			writeError(c, 401, "unauthorized", "authentication required")
			return
		}
		in, err := decodeStaff(c.Request.Body)
		if err != nil {
			writeError(c, 400, "invalid_input", "provide only email, password, display_name and role as required strings")
			return
		}
		user, err := reg.CreateStaff(c.Request.Context(), in.Email, in.Password, in.DisplayName, in.Role)
		switch {
		case errors.Is(err, service.ErrInvalidInput):
			writeError(c, 400, "invalid_input", err.Error())
		case errors.Is(err, service.ErrEmailExists):
			writeError(c, 409, "email_exists", "email already registered")
		case err != nil:
			writeError(c, 500, "internal_error", "internal server error")
		default:
			logger.InfoContext(c.Request.Context(), "staff account created", "request_id", c.GetString("request_id"), "actor_id", actor.ID, "new_user_id", user.ID, "role", user.Role)
			c.JSON(http.StatusCreated, dto.NewUserResponse(user))
		}
	}
}
