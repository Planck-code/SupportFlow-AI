//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Planck-code/SupportFlow-AI/internal/auth"
	"github.com/Planck-code/SupportFlow-AI/internal/config"
	"github.com/Planck-code/SupportFlow-AI/internal/database"
	"github.com/Planck-code/SupportFlow-AI/internal/dto"
	"github.com/Planck-code/SupportFlow-AI/internal/middleware"
	"github.com/Planck-code/SupportFlow-AI/internal/model"
	"github.com/Planck-code/SupportFlow-AI/internal/repository"
	"github.com/Planck-code/SupportFlow-AI/internal/router"
	"github.com/Planck-code/SupportFlow-AI/internal/service"
)

func TestAuthenticationMySQL(t *testing.T) {
	db, runner := testDatabase(t)
	if err := runner.Up(); err != nil {
		t.Fatal("migration failed")
	}
	assertVersion(t, runner, 2)
	users := repository.NewUsers(db)
	tokens, err := auth.NewJWT(strings.Repeat("integration-test-", 3))
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	login, err := service.NewLogin(users, tokens)
	if err != nil {
		t.Fatal(err)
	}
	r := router.New(logger, pool, service.NewRegistration(users), login, middleware.Authenticate(tokens, users))
	request := func(method, path string, body any, token string) *httptest.ResponseRecorder {
		t.Helper()
		data, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if strings.Contains(w.Body.String(), "password") {
			t.Fatal("sensitive response")
		}
		return w
	}
	credentials := map[string]string{"email": "login@example.com", "password": " actual-test-password ", "display_name": "Login"}
	w := request("POST", "/api/v1/auth/register", credentials, "")
	if w.Code != 201 {
		t.Fatal("registration failed")
	}
	var user dto.UserResponse
	if json.Unmarshal(w.Body.Bytes(), &user) != nil {
		t.Fatal("invalid user JSON")
	}
	credentials["email"] = "  LOGIN@Example.COM "
	w = request("POST", "/api/v1/auth/login", credentials, "")
	if w.Code != 200 {
		t.Fatal("normalized login failed")
	}
	var result dto.LoginResponse
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.ExpiresIn != 7200 || result.TokenType != "Bearer" {
		t.Fatal("incorrect token response")
	}
	if id, err := tokens.Validate(result.AccessToken); err != nil || id != user.ID {
		t.Fatal("invalid JWT identity")
	}
	for _, email := range []string{"login@example.com", "unknown@example.com"} {
		credentials["email"] = email
		credentials["password"] = "wrong"
		if w := request("POST", "/api/v1/auth/login", credentials, ""); w.Code != 401 {
			t.Fatal("bad credentials accepted")
		}
	}
	if w := request("GET", "/api/v1/users/me", nil, ""); w.Code != 401 {
		t.Fatal("anonymous access accepted")
	}
	if err := db.Model(&model.User{}).Where("id = ?", user.ID).Updates(map[string]any{"role": "agent", "display_name": "Current DB Name"}).Error; err != nil {
		t.Fatal("test user update failed")
	}
	w = request("GET", "/api/v1/users/me", nil, result.AccessToken)
	var current dto.UserResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &current) != nil || current.ID != user.ID || current.Role != "agent" || current.DisplayName != "Current DB Name" {
		t.Fatal("current DB identity not loaded")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := users.FindByID(canceled, user.ID); !errors.Is(err, context.Canceled) {
		t.Fatal("ID lookup ignored canceled context")
	}
	if _, err := users.FindByEmail(canceled, user.Email); !errors.Is(err, context.Canceled) {
		t.Fatal("email lookup ignored canceled context")
	}
	// This row belongs only to this test in the verified dedicated schema.
	if err := db.Delete(&model.User{}, user.ID).Error; err != nil {
		t.Fatal("test row delete failed")
	}
	if w := request("GET", "/api/v1/users/me", nil, result.AccessToken); w.Code != 401 {
		t.Fatal("deleted user token accepted")
	}
	// A separate, closed real SQL pool exercises database failures without stopping MySQL
	// or interfering with the original pool needed by the safe cleanup helper.
	cfg, err := config.LoadDatabase()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("SELECT DATABASE()").Scan(&cfg.Name).Error; err != nil {
		t.Fatal("test database lookup failed")
	}
	failedDB, err := database.Open(cfg)
	if err != nil {
		t.Fatal("pool setup failed")
	}
	failedPool, _ := failedDB.DB()
	failedPool.Close()
	failedUsers := repository.NewUsers(failedDB)
	failedLogin, err := service.NewLogin(failedUsers, tokens)
	if err != nil {
		t.Fatal(err)
	}
	r = router.New(logger, failedPool, service.NewRegistration(failedUsers), failedLogin, middleware.Authenticate(tokens, failedUsers))
	credentials["email"] = "login@example.com"
	credentials["password"] = " actual-test-password "
	if w := request("POST", "/api/v1/auth/login", credentials, ""); w.Code != 500 {
		t.Fatal("database error misreported as bad password")
	}
	if w := request("GET", "/api/v1/users/me", nil, result.AccessToken); w.Code != 500 {
		t.Fatal("database error misreported as invalid token")
	}
	if w := request("GET", "/healthz", nil, ""); w.Code != 200 {
		t.Fatal("health depends on database")
	}
	if w := request("GET", "/readyz", nil, ""); w.Code != 503 {
		t.Fatal("readiness ignores closed database")
	}
	t.Log("real MySQL: registration/login/current role/deleted user/context/closed-pool failures verified")
}
