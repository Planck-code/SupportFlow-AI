//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/Planck-code/SupportFlow-AI/internal/model"
	"github.com/Planck-code/SupportFlow-AI/internal/repository"
	"github.com/Planck-code/SupportFlow-AI/internal/service"
)

func TestStaffCreationLiveMySQL(t *testing.T) {
	db, runner := testDatabase(t)
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, runner, 3)
	users := repository.NewUsers(db)
	reg := service.NewRegistration(users)
	ctx := context.Background()
	initialPassword := rand.Text()
	actor, err := reg.CreateStaff(ctx, "initial@example.test", initialPassword, "Initial", "admin")
	if err != nil {
		t.Fatal("cannot seed first administrator")
	}
	base, logPath := startTestAPI(t, os.Getenv("TEST_DATABASE_NAME"))
	call := liveRequests(t, base+"/api/v1")
	login := func(email, password string) string {
		t.Helper()
		result := call("POST", "/auth/login", "", map[string]string{"email": email, "password": password}, 200)
		token, ok := result["access_token"].(string)
		if !ok || token == "" {
			t.Fatal("missing login token")
		}
		return token
	}
	adminToken := login(actor.Email, initialPassword)
	agentPassword := " " + strings.Repeat("界", 23) + "  "
	agentBody := map[string]string{"email": "  AGENT@example.test ", "password": agentPassword, "display_name": " " + strings.Repeat("王", 64) + " ", "role": "agent"}
	post := func(token string, body any, want int) map[string]any {
		t.Helper()
		return call("POST", "/admin/staff", token, body, want)
	}
	post("", agentBody, 401)
	post("invalid-token", agentBody, 401)
	created := post(adminToken, agentBody, 201)
	verifyUser := func(m map[string]any, role, email, name string) {
		t.Helper()
		if len(m) != 5 || m["id"] == nil || m["created_at"] == nil || m["role"] != role || m["email"] != email || m["display_name"] != name {
			t.Fatal("unexpected user DTO")
		}
	}
	verifyUser(created, "agent", "agent@example.test", strings.Repeat("王", 64))
	agent, err := users.FindByEmail(ctx, "agent@example.test")
	if err != nil || bcrypt.CompareHashAndPassword([]byte(agent.PasswordHash), []byte(agentPassword)) != nil || created["id"] != float64(agent.ID) {
		t.Fatal("stored agent incorrect")
	}
	agentToken := login(agent.Email, agentPassword)
	post(agentToken, agentBody, 403)
	adminPassword := rand.Text()
	adminBody := map[string]string{"email": "second@example.test", "password": adminPassword, "display_name": "Second", "role": "admin"}
	second := post(adminToken, adminBody, 201)
	verifyUser(second, "admin", adminBody["email"], "Second")
	secondToken := login(adminBody["email"], adminPassword)
	for _, token := range []string{adminToken, secondToken} {
		list := call("GET", "/admin/agents", token, nil, 200)
		items, ok := list["items"].([]any)
		if !ok || len(items) != 1 || list["total"] != 1.0 || items[0].(map[string]any)["id"] != created["id"] {
			t.Fatal("agent not visible in admin directory")
		}
	}
	for _, role := range []string{"user", "owner"} {
		post(adminToken, map[string]string{"email": "invalid@example.test", "password": adminPassword, "display_name": "Test", "role": role}, 400)
	}
	for _, field := range []string{"email", "password", "display_name", "role"} {
		data := map[string]string{}
		for k, v := range adminBody {
			if k != field {
				data[k] = v
			}
		}
		post(adminToken, data, 400)
	}
	for field, value := range map[string]string{"email": "bad address", "password": agentPassword + "x", "display_name": strings.Repeat("界", 65), "actor_id": "9999"} {
		data := map[string]string{}
		for k, v := range adminBody {
			data[k] = v
		}
		data[field] = value
		post(adminToken, data, 400)
	}
	adminBody["email"] = agent.Email
	post(adminToken, adminBody, 409)
	unchanged, err := users.FindByEmail(ctx, agent.Email)
	if err != nil || unchanged != agent {
		t.Fatal("duplicate overwrote existing record")
	}
	// Force the INSERT to fail after the actor's DB authentication succeeded.
	if err := db.Exec("ALTER TABLE users ADD CONSTRAINT test_fail_staff CHECK (email <> 'blocked@example.test')").Error; err != nil {
		t.Fatal("cannot install test constraint")
	}
	adminBody["email"] = "blocked@example.test"
	failure := post(adminToken, adminBody, 500)
	if failure["code"] != "internal_error" || failure["message"] != "internal server error" {
		t.Fatal("unsafe database error")
	}
	for _, role := range []string{"agent", "admin"} {
		adminBody["role"] = role
		call("POST", "/auth/register", "", adminBody, 400)
	}
	if _, err := reg.Register(ctx, "ordinary@example.test", initialPassword, "Ordinary"); err != nil {
		t.Fatal(err)
	}
	ordinaryToken := login("ordinary@example.test", initialPassword)
	post(ordinaryToken, adminBody, 403)
	if err := db.Model(&model.User{}).Where("id = ?", actor.ID).Update("role", "user").Error; err != nil {
		t.Fatal("test demotion failed")
	}
	post(adminToken, adminBody, 403)
	assertVersion(t, runner, 3)

	logs, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{initialPassword, adminPassword, agentPassword, agent.PasswordHash, adminToken, secondToken, agentToken, ordinaryToken, os.Getenv("DB_PASSWORD"), "blocked@example.test"} {
		if secret != "" && bytes.Contains(logs, []byte(secret)) {
			t.Fatal("sensitive API log")
		}
	}
	// Match each success audit entry to the request log for its own HTTP 201.
	audits := map[string]map[string]any{}
	successful := map[string]bool{}
	for _, line := range bytes.Split(bytes.TrimSpace(logs), []byte("\n")) {
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatal("invalid JSON log")
		}
		id, _ := entry["request_id"].(string)
		if entry["msg"] == "staff account created" {
			if id == "" || entry["actor_id"] != float64(actor.ID) || len(entry) != 7 {
				t.Fatal("incorrect audit metadata")
			}
			audits[id] = entry
		}
		if entry["msg"] == "http request" && entry["route"] == "/api/v1/admin/staff" && entry["status"] == 201.0 {
			successful[id] = true
		}
	}
	if len(audits) != 2 || len(successful) != 2 {
		t.Fatal("expected two successful staff audit records")
	}
	expected := map[any]string{created["id"]: "agent", second["id"]: "admin"}
	for id, entry := range audits {
		role, ok := expected[entry["new_user_id"]]
		if !successful[id] || !ok || entry["role"] != role {
			t.Fatal("audit not linked to created user and request")
		}
		delete(expected, entry["new_user_id"])
	}
	t.Log("real API: both new roles logged in, directory visible to both admins; 401/403/400/409/500, demotion, bcrypt and audit verified")
}
