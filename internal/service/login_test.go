package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Planck-code/SupportFlow-AI/internal/auth"
	"github.com/Planck-code/SupportFlow-AI/internal/model"
	"github.com/Planck-code/SupportFlow-AI/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

type missingLoginUser struct{}

func (missingLoginUser) FindByEmail(context.Context, string) (model.User, error) {
	return model.User{}, repository.ErrUserNotFound
}

func TestMissingUserStillVerifiesHash(t *testing.T) {
	tokens, err := auth.NewJWT(strings.Repeat("unit-test-", 4))
	if err != nil {
		t.Fatal(err)
	}
	login, err := NewLogin(missingLoginUser{}, tokens)
	if err != nil {
		t.Fatal(err)
	}
	if cost, err := bcrypt.Cost(login.dummyHash); err != nil || cost != bcrypt.DefaultCost {
		t.Fatal("dummy hash must use registration cost")
	}
	if token, err := login.Login(context.Background(), "missing@example.com", "wrong-password"); !errors.Is(err, ErrInvalidCredentials) || token != "" {
		t.Fatal("missing user must not receive a token")
	}
	matchingHash, err := bcrypt.GenerateFromPassword([]byte("known-test-input"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	login.dummyHash = matchingHash
	if token, err := login.Login(context.Background(), "missing@example.com", "known-test-input"); !errors.Is(err, ErrInvalidCredentials) || token != "" {
		t.Fatal("matching dummy hash must never authenticate a nonexistent user")
	}
	// A malformed comparison hash must surface as an internal error. If the
	// not-found branch skips comparison, this returns ErrInvalidCredentials instead.
	login.dummyHash = []byte("malformed")
	if _, err := login.Login(context.Background(), "missing@example.com", "wrong-password"); err == nil || errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("missing user skipped bcrypt comparison")
	}
}
