//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Planck-code/SupportFlow-AI/internal/auth"
	"github.com/Planck-code/SupportFlow-AI/internal/dto"
	"github.com/Planck-code/SupportFlow-AI/internal/middleware"
	"github.com/Planck-code/SupportFlow-AI/internal/model"
	"github.com/Planck-code/SupportFlow-AI/internal/repository"
	"github.com/Planck-code/SupportFlow-AI/internal/router"
	"github.com/Planck-code/SupportFlow-AI/internal/service"
	"golang.org/x/crypto/bcrypt"
)

func TestStaffDirectoryMySQL(t *testing.T) {
	db, runner := testDatabase(t)
	if err := runner.Up(); err != nil {
		t.Fatal("test migrations failed")
	}
	binary := filepath.Join(t.TempDir(), "staff-cli.exe")
	if out, err := exec.Command("go", "build", "-o", binary, "../../cmd/admin").CombinedOutput(); err != nil {
		t.Fatalf("CLI build: %s", out)
	}
	password := rand.Text()
	create := func(role, email, pass string, extra ...string) (string, error) {
		args := []string{"-role", role, "-email", email, "-display-name", " Staff "}
		cmd := exec.Command(binary, append(args, extra...)...)
		cmd.Env = append(os.Environ(), "DB_NAME="+os.Getenv("TEST_DATABASE_NAME"), "STAFF_PASSWORD="+pass)
		out, err := cmd.CombinedOutput()
		if strings.Contains(string(out), pass) || strings.Contains(string(out), password) || strings.Contains(string(out), os.Getenv("DB_PASSWORD")) || strings.Contains(string(out), "$2a$") {
			t.Fatal("sensitive CLI output")
		}
		return string(out), err
	}
	users := repository.NewUsers(db)
	ctx := context.Background()
	ids := map[string]uint64{}
	for _, account := range []struct{ role, email string }{{"admin", "boss@example.com"}, {"agent", "first@example.com"}, {"agent", "second@example.com"}} {
		if _, err := create(account.role, " "+strings.ToUpper(account.email)+" ", password); err != nil {
			t.Fatal("CLI create failed")
		}
		u, err := users.FindByEmail(ctx, account.email)
		if err != nil || u.Role != account.role || u.DisplayName != "Staff" || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
			t.Fatal("incorrect CLI record")
		}
		ids[account.email] = u.ID
	}
	if out, err := create("agent", "boss@example.com", rand.Text()); err == nil || !strings.Contains(out, "email already registered") {
		t.Fatal("duplicate must fail explicitly")
	}
	boss, err := users.FindByEmail(ctx, "boss@example.com")
	if err != nil || boss.Role != "admin" || bcrypt.CompareHashAndPassword([]byte(boss.PasswordHash), []byte(password)) != nil {
		t.Fatal("duplicate overwrote existing user")
	}
	for _, role := range []string{"user", "owner"} {
		if _, err := create(role, "invalid@example.com", password); err == nil {
			t.Fatal("invalid staff role accepted")
		}
	}
	if _, err := create("admin", "invalid@example.com", password, "-password", password); err == nil {
		t.Fatal("CLI accepted password argument")
	}
	var count int64
	if err := db.Model(&model.User{}).Count(&count).Error; err != nil || count != 3 {
		t.Fatal("rejected CLI mutated accounts")
	}
	reg := service.NewRegistration(users)
	ordinary, err := reg.Register(ctx, "user@example.com", password, "User")
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := auth.NewJWT(rand.Text() + rand.Text())
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	var logs bytes.Buffer
	r := router.New(slog.New(slog.NewJSONHandler(&logs, nil)), pool, reg, nil, middleware.Authenticate(tokens, users), users)
	issue := func(id uint64) string {
		raw, err := tokens.Issue(id)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	adminToken := issue(boss.ID)
	agentToken := issue(ids["first@example.com"])
	userToken := issue(ordinary.ID)
	get := func(token, query string, want int) []byte {
		t.Helper()
		req := httptest.NewRequest("GET", "/api/v1/admin/agents"+query, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("status %d want %d", w.Code, want)
		}
		for _, secret := range []string{password, boss.PasswordHash, adminToken, agentToken, userToken, os.Getenv("DB_PASSWORD"), "password_hash"} {
			if strings.Contains(w.Body.String(), secret) || strings.Contains(logs.String(), secret) {
				t.Fatal("sensitive response or log")
			}
		}
		return w.Body.Bytes()
	}
	get("", "", 401)
	get("invalid", "", 401)
	get(userToken, "?role=admin", 403)
	get(agentToken, "", 403)
	for _, tc := range []struct {
		query      string
		page, size int
		want       []uint64
	}{
		{"", 1, 20, []uint64{ids["first@example.com"], ids["second@example.com"]}},
		{"?page_size=1&page=2", 2, 1, []uint64{ids["second@example.com"]}},
		{"?page_size=100&page=2", 2, 100, []uint64{}},
	} {
		var result struct {
			Items    []dto.UserResponse
			Total    int
			Page     int
			PageSize int `json:"page_size"`
		}
		if json.Unmarshal(get(adminToken, tc.query, 200), &result) != nil || result.Items == nil || result.Total != 2 || result.Page != tc.page || result.PageSize != tc.size || len(result.Items) != len(tc.want) {
			t.Fatal("incorrect page")
		}
		for i, id := range tc.want {
			if result.Items[i].ID != id || result.Items[i].Role != "agent" {
				t.Fatal("incorrect list order or filtering")
			}
		}
	}
	if err := db.Model(&model.User{}).Where("id = ?", boss.ID).Update("role", "user").Error; err != nil {
		t.Fatal("test role update failed")
	}
	get(adminToken, "", 403)
}
