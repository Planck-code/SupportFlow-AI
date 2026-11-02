package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Planck-code/SupportFlow-AI/internal/model"
)

type captureUsers struct {
	user  model.User
	err   error
	calls int
	ctx   context.Context
}

func (r *captureUsers) Create(ctx context.Context, user *model.User) error {
	r.calls++
	r.ctx = ctx
	r.user = *user
	user.ID = 42
	user.CreatedAt = time.Now().UTC()
	return r.err
}

func TestRegisterNormalizesAndHashes(t *testing.T) {
	users := &captureUsers{}
	svc := NewRegistration(users)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	password := " password-with-spaces "
	user, err := svc.Register(ctx, "  TeSt@Example.COM  ", password, "  测试用户  ")
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != 42 || user.Email != "test@example.com" || user.DisplayName != "测试用户" || user.Role != "user" {
		t.Fatal("unexpected registered user")
	}
	if users.ctx != ctx || users.calls != 1 {
		t.Fatal("context or insertion count incorrect")
	}
	if users.user.PasswordHash == password || bcrypt.CompareHashAndPassword([]byte(users.user.PasswordHash), []byte(password)) != nil {
		t.Fatal("password not safely hashed or silently modified")
	}
	if cost, err := bcrypt.Cost([]byte(users.user.PasswordHash)); err != nil || cost != bcrypt.DefaultCost {
		t.Fatal("unexpected bcrypt cost")
	}
}

func TestRegisterValidation(t *testing.T) {
	for _, tt := range []struct {
		name, email, password, display string
		valid                          bool
	}{
		{"valid", "test@example.com", "12345678", "Test", true},
		{"empty email", "", "12345678", "Test", false},
		{"bad email", "invalid", "12345678", "Test", false},
		{"display address", "Name <test@example.com>", "12345678", "Test", false},
		{"long email", strings.Repeat("a", 250) + "@test.com", "12345678", "Test", false},
		{"blank display", "test@example.com", "12345678", " \t ", false},
		{"64 unicode display", "test@example.com", "12345678", strings.Repeat("名", 64), true},
		{"65 unicode display", "test@example.com", "12345678", strings.Repeat("名", 65), false},
		{"short password", "test@example.com", "1234567", "Test", false},
		{"72 bytes", "test@example.com", strings.Repeat("a", 72), "Test", true},
		{"73 bytes", "test@example.com", strings.Repeat("a", 73), "Test", false},
		{"unicode 72 bytes", "test@example.com", strings.Repeat("密", 24), "Test", true},
		{"unicode 75 bytes", "test@example.com", strings.Repeat("密", 25), "Test", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			users := &captureUsers{}
			_, err := NewRegistration(users).Register(context.Background(), tt.email, tt.password, tt.display)
			if tt.valid {
				if err != nil || users.calls != 1 {
					t.Fatalf("valid registration failed: %v", err)
				}
			} else if !errors.Is(err, ErrInvalidInput) || users.calls != 0 {
				t.Fatal("invalid input reached storage or was accepted")
			}
		})
	}
}

func TestRegisterPropagatesStorageErrors(t *testing.T) {
	for _, storageErr := range []error{ErrEmailExists, errors.New("private storage error")} {
		_, err := NewRegistration(&captureUsers{err: storageErr}).Register(context.Background(), "test@example.com", "12345678", "Test")
		if !errors.Is(err, storageErr) {
			t.Fatal("storage error was lost")
		}
	}
}
