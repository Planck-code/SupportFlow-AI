package service

import (
	"context"
	"crypto/rand"
	"errors"

	"github.com/Planck-code/SupportFlow-AI/internal/auth"
	"github.com/Planck-code/SupportFlow-AI/internal/model"
	"github.com/Planck-code/SupportFlow-AI/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidCredentials = errors.New("invalid email or credentials")

type userByEmail interface {
	FindByEmail(context.Context, string) (model.User, error)
}

type Login struct {
	users     userByEmail
	tokens    *auth.JWT
	dummyHash []byte
}

func NewLogin(users userByEmail, tokens *auth.JWT) (*Login, error) {
	// Match registration's bcrypt work even when no account exists. Generate once
	// at startup, never per request; this hash is not a stored user credential.
	hash, err := bcrypt.GenerateFromPassword([]byte(rand.Text()), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return &Login{users: users, tokens: tokens, dummyHash: hash}, nil
}

func (s *Login) Login(ctx context.Context, email, password string) (string, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return "", err
	}
	user, err := s.users.FindByEmail(ctx, email)
	missing := errors.Is(err, repository.ErrUserNotFound)
	if err != nil && !missing {
		return "", err
	}
	// Reject unsupported lengths explicitly; bcrypt comparison must not accept a
	// longer input that shares the first 72 bytes with a registered password.
	if len(password) < 8 || len(password) > 72 {
		return "", ErrInvalidCredentials
	}
	hash := []byte(user.PasswordHash)
	if missing {
		hash = s.dummyHash
	}
	err = bcrypt.CompareHashAndPassword(hash, []byte(password))
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return "", ErrInvalidCredentials
	}
	if err != nil {
		return "", err
	}
	if missing {
		return "", ErrInvalidCredentials
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return s.tokens.Issue(user.ID)
}
