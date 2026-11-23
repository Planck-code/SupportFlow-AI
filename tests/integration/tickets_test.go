//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Planck-code/SupportFlow-AI/internal/model"
	"github.com/Planck-code/SupportFlow-AI/internal/repository"
	"github.com/Planck-code/SupportFlow-AI/internal/service"
)

func TestTicketHTTPMySQL(t *testing.T) {
	db, runner := testDatabase(t)
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, runner, latestVersion)
	base, _ := startTestAPI(t, os.Getenv("TEST_DATABASE_NAME"))
	call := liveRequests(t, base+"/api/v1")
	reg := service.NewRegistration(repository.NewUsers(db))
	password := rand.Text()
	ids := map[string]uint64{}
	tokens := map[string]string{}
	for name, role := range map[string]string{"rollback": "user", "alice": "user", "bob": "user", "agent": "agent", "otherAgent": "agent", "admin": "admin"} {
		email := name + "@example.test"
		var u model.User
		var err error
		if role == "user" {
			u, err = reg.Register(context.Background(), email, password, name)
		} else {
			u, err = reg.CreateStaff(context.Background(), email, password, name, role)
		}
		if err != nil {
			t.Fatal("could not seed test identity")
		}
		ids[name] = u.ID
		result := call("POST", "/auth/login", "", map[string]string{"email": email, "password": password}, 200)
		tokens[name] = result["access_token"].(string)
	}
	create := func(who string, body any, want int) map[string]any {
		t.Helper()
		return call("POST", "/tickets", tokens[who], body, want)
	}
	get := func(who, path string, want int) map[string]any {
		t.Helper()
		return call("GET", "/tickets"+path, tokens[who], nil, want)
	}
	body := map[string]any{"title": "申请帮助", "description": "private ticket text", "requester_id": ids["bob"], "assignee_id": ids["agent"], "priority": "high", "status": "closed"}
	create("", body, 401)
	create("agent", body, 403)
	create("admin", body, 403)
	for _, bad := range []map[string]string{{"title": " ", "description": "ok"}, {"title": "ok", "description": " \n"}, {"title": strings.Repeat("字", 121), "description": "ok"}, {"title": "ok", "description": strings.Repeat("字", 10001)}, {"title": "ok", "description": "ok", "category": "other"}} {
		create("alice", bad, 400)
	}
	create("alice", map[string]any{"title": "ok", "description": "ok", "category": nil}, 400)
	ticket := create("alice", body, 201)
	if ticket["requester_id"] != float64(ids["alice"]) || ticket["assignee_id"] != nil || ticket["priority"] != "normal" || ticket["status"] != "open" || ticket["category"] != "general" {
		t.Fatal("incorrect defaults or trusted client fields")
	}
	id := uint64(ticket["id"].(float64))
	path := fmt.Sprintf("/%d", id)
	second := create("alice", map[string]string{"title": strings.Repeat("界", 120), "description": strings.Repeat("界", 10000), "category": "technical"}, 201)
	secondID := uint64(second["id"].(float64))
	create("bob", map[string]string{"title": "Other", "description": "Bob", "category": "billing"}, 201)
	detail := get("alice", path, 200)
	if detail["description"] != body["description"] {
		t.Fatal("owner detail mismatch")
	}
	get("admin", path, 200)
	for _, who := range []string{"bob", "agent", "otherAgent"} {
		get(who, path, 404)
		get(who, path+"/events", 404)
	}
	for _, suffix := range []string{"", "/events"} {
		get("admin", "/999999"+suffix, 404)
	}
	events := get("alice", path+"/events", 200)
	event := events["items"].([]any)[0].(map[string]any)
	if events["total"] != 1.0 || event["event_type"] != "created" || event["actor_id"] != float64(ids["alice"]) || event["ticket_id"] != float64(id) || event["metadata"].(map[string]any)["status"] != "open" {
		t.Fatal("incorrect creation event")
	}
	meta, _ := json.Marshal(event["metadata"])
	if bytes.Contains(meta, []byte("private ticket text")) {
		t.Fatal("event contains description")
	}
	for _, suffix := range []string{"", "/queue", path, path + "/events"} {
		get("", suffix, 401)
	}
	get("alice", "/queue", 403)
	get("admin", "/queue", 403)
	get("agent", "/queue?status=open", 400)
	queue := get("agent", "/queue?category=technical", 200)
	qitem := queue["items"].([]any)[0].(map[string]any)
	if queue["total"] != 1.0 || len(qitem) != 5 {
		t.Fatal("queue count or projection incorrect")
	}
	for _, k := range []string{"id", "title", "category", "priority", "created_at"} {
		if _, ok := qitem[k]; !ok {
			t.Fatal("missing queue field", k)
		}
	}

	// Seed an assigned ticket directly in the test schema to exercise agent visibility.
	if err := db.Model(&model.Ticket{}).Where("id = ?", id).Updates(map[string]any{"status": "in_progress", "assignee_id": ids["agent"]}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.TicketEvent{TicketID: id, ActorID: ids["admin"], EventType: "assigned", Metadata: []byte("{}")}).Error; err != nil {
		t.Fatal(err)
	}
	get("agent", path, 200)
	get("otherAgent", path, 404)
	get("otherAgent", path+"/events", 404)
	e2 := get("agent", path+"/events?page=2&page_size=1", 200)
	if e2["total"] != 2.0 || e2["items"].([]any)[0].(map[string]any)["event_type"] != "assigned" {
		t.Fatal("event pagination incorrect")
	}
	if err := db.Model(&model.Ticket{}).Where("requester_id = ?", ids["alice"]).Update("created_at", time.Now().UTC()).Error; err != nil {
		t.Fatal(err)
	}
	first := get("alice", "?page_size=1", 200)
	next := get("alice", "?page=2&page_size=1", 200)
	if first["total"] != 2.0 || first["items"].([]any)[0].(map[string]any)["id"] != float64(secondID) || next["items"].([]any)[0].(map[string]any)["id"] != float64(id) {
		t.Fatal("unstable list pagination")
	}
	for _, tc := range []struct {
		who, q string
		total  float64
	}{
		{"alice", "?category=billing", 0}, {"agent", "", 1}, {"otherAgent", "", 0}, {"admin", "", 3}, {"admin", "?unassigned=true", 2}, {"agent", "/queue", 2}, {"admin", fmt.Sprintf("?status=in_progress&assignee_id=%d", ids["agent"]), 1},
	} {
		result := get(tc.who, tc.q, 200)
		if result["total"] != tc.total || float64(len(result["items"].([]any))) != tc.total || result["page_size"] != 20.0 {
			t.Fatal("incorrect filtered page", tc)
		}
	}
	for _, q := range []string{"/abc", "/0", "/-1/events", "/18446744073709551616", "?page=0", "?page_size=101", "?status=bad", "?category=wrong", "?unassigned=false", "?assignee_id=x", "?unassigned=true&assignee_id=2", "?category=general&category=billing", path + "/events?category=general"} {
		get("admin", q, 400)
	}
	get("alice", "?assignee_id=1", 400)

	// A real MySQL failure on the second INSERT must roll back the first.
	constraint := fmt.Sprintf("ALTER TABLE ticket_events ADD CONSTRAINT test_reject_actor CHECK (actor_id <> %d)", ids["rollback"])
	if err := db.Exec(constraint).Error; err != nil {
		t.Fatal("could not install test-only constraint", err)
	}
	failure := create("rollback", body, 500)
	if failure["code"] != "internal_error" || failure["message"] != "internal server error" {
		t.Fatal("database error exposed")
	}
	for table, want := range map[string]int64{"tickets": 3, "ticket_events": 4} {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil || count != want {
			t.Fatal("partial transaction", table)
		}
	}
	t.Log("live cmd/api + MySQL: five routes, ownership, queue projection, stable pages, forced event failure and transaction rollback passed")
}
