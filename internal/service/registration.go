package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"github.com/Planck-code/SupportFlow-AI/internal/model"
	"github.com/Planck-code/SupportFlow-AI/internal/repository"
)

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrEmailExists  = repository.ErrEmailExists
)

// Define only the storage capability this service consumes, for isolated tests.
type userCreator interface {
	Create(context.Context, *model.User) error
}

type Registration struct{ users userCreator }

func NewRegistration(users userCreator) *Registration { return &Registration{users: users} }

func (s *Registration) Register(ctx context.Context, email, password, displayName string) (model.User, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return model.User{}, err
	}
	displayName = strings.TrimSpace(displayName)
	if count := utf8.RuneCountInString(displayName); count < 1 || count > 64 {
		return model.User{}, fmt.Errorf("%w: display_name must contain 1 to 64 characters", ErrInvalidInput)
	}
	// bcrypt's limit is bytes, not Unicode characters. Never trim the password.
	if len(password) < 8 || len(password) > 72 {
		return model.User{}, fmt.Errorf("%w: password must contain 8 to 72 bytes", ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return model.User{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return model.User{}, err
	}
	user := model.User{Email: email, PasswordHash: string(hash), DisplayName: displayName, Role: model.RoleUser}
	if err := s.users.Create(ctx, &user); err != nil {
		return model.User{}, err
	}
	return user, nil
}

func normalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	address, err := mail.ParseAddress(email)
	if err != nil || len(email) > 254 || address.Address != email || address.Name != "" {
		return "", fmt.Errorf("%w: email must be a valid address of at most 254 bytes", ErrInvalidInput)
	}
	return email, nil
}
