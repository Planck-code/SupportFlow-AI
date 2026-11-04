//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Planck-code/SupportFlow-AI/internal/dto"
	"github.com/Planck-code/SupportFlow-AI/internal/model"
	"github.com/Planck-code/SupportFlow-AI/internal/repository"
	"github.com/Planck-code/SupportFlow-AI/internal/router"
	"github.com/Planck-code/SupportFlow-AI/internal/service"
)

func TestRegistrationMySQL(t *testing.T) {
	db, runner := testDatabase(t)
	if err := runner.Up(); err != nil {
		t.Fatal("migration failed")
	}
	assertVersion(t, runner, 2)
	pool, _ := db.DB()
	users := repository.NewUsers(db)
	registration := service.NewRegistration(users)
	var logs bytes.Buffer
	r := router.New(slog.New(slog.NewJSONHandler(&logs, nil)), pool, registration, nil, nil)
	password := " test-password-preserved "
	email := "register@example.com"
	request := func(address string) (int, dto.UserResponse) {
		body, _ := json.Marshal(map[string]string{"email": address, "password": password, "display_name": " 测试用户 "})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if strings.Contains(w.Body.String(), password) || strings.Contains(w.Body.String(), "password_hash") {
			t.Error("sensitive registration response")
		}
		var result dto.UserResponse
		if w.Code == 201 && json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Error("invalid success response")
		}
		return w.Code, result
	}
	status, result := request("  REGISTER@Example.COM  ")
	if status != 201 || result.ID == 0 || result.Email != email || result.Role != "user" || result.DisplayName != "测试用户" {
		t.Fatal("real registration failed or DTO incorrect")
	}
	var stored model.User
	if err := db.First(&stored, result.ID).Error; err != nil {
		t.Fatal("registered user missing")
	}
	if stored.PasswordHash == password || bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte(password)) != nil {
		t.Fatal("password not stored as verifiable bcrypt hash")
	}
	if stored.Email != email || stored.Role != "user" || stored.DisplayName != "测试用户" {
		t.Fatal("stored user fields incorrect")
	}
	if stored.CreatedAt.IsZero() || stored.UpdatedAt.IsZero() || time.Since(stored.CreatedAt) > time.Minute || time.Until(stored.CreatedAt) > time.Minute {
		t.Fatal("creation timestamps not current UTC")
	}
	if status, _ := request(email); status != 409 {
		t.Fatal("duplicate email did not return 409")
	}
	if status, _ := request(strings.ToUpper(email)); status != 409 {
		t.Fatal("case variant did not return 409")
	}
	// Bypass service normalization to prove the MySQL index itself rejects case variants.
	duplicate := model.User{Email: strings.ToUpper(email), DisplayName: "Duplicate", Role: "user", PasswordHash: stored.PasswordHash}
	if err := users.Create(context.Background(), &duplicate); !errors.Is(err, repository.ErrEmailExists) {
		t.Fatal("database unique index or 1062 mapping failed")
	}
	invalidRole := model.User{Email: "badrole@example.com", DisplayName: "Role", Role: "ADMIN", PasswordHash: stored.PasswordHash}
	if err := users.Create(context.Background(), &invalidRole); err == nil || errors.Is(err, repository.ErrEmailExists) {
		t.Fatal("role constraint failed or non-duplicate error misclassified")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := users.Create(canceled, &model.User{Email: "cancel@example.com", DisplayName: "Cancel", Role: "user", PasswordHash: stored.PasswordHash}); !errors.Is(err, context.Canceled) {
		t.Fatal("repository ignored context cancellation")
	}
	if strings.Contains(logs.String(), password) || strings.Contains(logs.String(), stored.PasswordHash) {
		t.Fatal("registration log leaked credentials")
	}

	t.Run("concurrent same email", func(t *testing.T) {
		const workers = 8
		start := make(chan struct{})
		statuses := make(chan int, workers)
		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); <-start; status, _ := request("race@example.com"); statuses <- status }()
		}
		close(start)
		wg.Wait()
		close(statuses)
		success, conflict := 0, 0
		for status := range statuses {
			switch status {
			case 201:
				success++
			case 409:
				conflict++
			default:
				t.Errorf("unexpected concurrent status %d", status)
			}
		}
		if success != 1 || conflict != workers-1 {
			t.Fatalf("success=%d conflict=%d, want 1/%d", success, conflict, workers-1)
		}
		var count int64
		if err := db.Model(&model.User{}).Where("email = ?", "race@example.com").Count(&count).Error; err != nil || count != 1 {
			t.Fatal("concurrent registration did not produce exactly one row")
		}
		t.Logf("%d concurrent HTTP registrations: %d created, %d conflicts, exactly one row", workers, success, conflict)
	})
}
