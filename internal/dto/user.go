package dto

import (
	"encoding/json"
	"time"

	"github.com/Planck-code/SupportFlow-AI/internal/model"
)

type RegisterRequest struct {
	Email       string `json:"email" binding:"required"`
	Password    string `json:"password" binding:"required"`
	DisplayName string `json:"display_name" binding:"required"`
	// RawMessage distinguishes an absent role from an explicitly supplied null.
	Role json.RawMessage `json:"role"`
}

type UserResponse struct {
	ID          uint64    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
}

func NewUserResponse(user model.User) UserResponse {
	return UserResponse{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName, Role: user.Role, CreatedAt: user.CreatedAt.UTC()}
}
