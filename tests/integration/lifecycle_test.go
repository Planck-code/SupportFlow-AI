//go:build integration

package integration

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/Planck-code/SupportFlow-AI/internal/repository"
	"github.com/Planck-code/SupportFlow-AI/internal/service"
)

type lifecycleTest struct {
	t      *testing.T
	db     *gorm.DB
	base   string
	ids    map[string]uint64
	tokens map[string]string
	call   func(string, string, string, any, int) map[string]any
}

func (f lifecycleTest) create(title string) string {
	f.t.Helper()
	ticket := f.call("POST", "/tickets", f.tokens["owner"], map[string]string{"title": title, "description": "private lifecycle description"}, 201)
	return fmt.Sprintf("/tickets/%.0f", ticket["id"])
}

func (f lifecycleTest) post(path, actor string, want int) map[string]any {
	f.t.Helper()
	return f.call("POST", path, f.tokens[actor], map[string]any{}, want)
}

func (f lifecycleTest) priority(path, actor, value string, want int) map[string]any {
	f.t.Helper()
	return f.call("PATCH", path+"/priority", f.tokens[actor], map[string]string{"priority": value}, want)
}

func (f lifecycleTest) detail(path string) map[string]any {
	return f.call("GET", path, f.tokens["owner"], nil, 200)
}

func (f lifecycleTest) history(path string) []any {
	return f.call("GET", path+"/events", f.tokens["owner"], nil, 200)["items"].([]any)
}

func (f lifecycleTest) processing(title string) string {
	path := f.create(title)
	f.post(path+"/claim", "a", 200)
	return path
}

func TestLifecycleLiveMySQL(t *testing.T) {
	db, runner := testDatabase(t)
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, runner, latestVersion)
	base, logPath := startTestAPI(t, os.Getenv("TEST_DATABASE_NAME"))
	base += "/api/v1"
	ids, tokens, pass := seedTicketActors(t, db, base)
	for _, test := range []struct {
		name string
		run  func(lifecycleTest)
	}{
		{"workflow_and_access", testLifecycleWorkflow},
		{"ordered_races", testLifecycleRaces},
		{"rollback", testLifecycleRollback},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.run(lifecycleTest{t, db, base, ids, tokens, liveRequests(t, base)})
		})
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	hidden := []string{pass, os.Getenv("DB_PASSWORD"), "private lifecycle description", "private lifecycle reply", "test_block_lifecycle", "UPDATE `tickets`"}
	for _, token := range tokens {
		hidden = append(hidden, token)
	}
	for _, v := range hidden {
		if v != "" && strings.Contains(string(log), v) {
			t.Fatal("sensitive data in API log")
		}
	}
}

func seedTicketActors(t *testing.T, db *gorm.DB, base string) (map[string]uint64, map[string]string, string) {
	t.Helper()
	call := liveRequests(t, base)
	reg := service.NewRegistration(repository.NewUsers(db))
	pass := rand.Text()
	ids := map[string]uint64{}
	tokens := map[string]string{}
	for name, role := range map[string]string{"admin": "admin", "owner": "user", "other": "user", "a": "agent", "b": "agent"} {
		email := name + "@example.test"
		if role == "user" {
			user, err := reg.Register(context.Background(), email, pass, name)
			if err != nil {
				t.Fatal("cannot seed ordinary user")
			}
			ids[name] = user.ID
		} else {
			user, err := reg.CreateStaff(context.Background(), email, pass, name, role)
			if err != nil {
				t.Fatal("cannot seed staff")
			}
			ids[name] = user.ID
		}
		tokens[name] = call("POST", "/auth/login", "", map[string]string{"email": email, "password": pass}, 200)["access_token"].(string)
	}
	return ids, tokens, pass
}

func testLifecycleWorkflow(f lifecycleTest) {
	t := f.t
	path := f.create("Lifecycle")
	// A hidden ticket's state must not be disclosed by an action.
	f.post(path+"/resolve", "a", 404)
	f.post(path+"/close", "owner", 409)
	f.priority(path, "admin", "normal", 409)
	f.post(path+"/claim", "a", 200)
	for _, tc := range []struct {
		action, actor string
		want          int
	}{
		{"resolve", "", 401}, {"resolve", "admin", 403}, {"resolve", "b", 404}, {"reopen", "a", 403},
		{"close", "a", 403}, {"close", "other", 404}, {"reopen", "other", 404},
	} {
		f.post(path+"/"+tc.action, tc.actor, tc.want)
	}
	f.post("/tickets/987654/resolve", "a", 404)
	f.priority(path, "owner", "high", 403)
	f.priority(path, "a", "urgent", 400)
	f.call("POST", path+"/reopen", f.tokens["owner"], map[string]string{"status": "in_progress"}, 400)
	f.call("POST", path+"/close", f.tokens["admin"], map[string]int{"actor_id": 1}, 400)
	reply := func(actor string, want int) {
		f.call("POST", path+"/messages", f.tokens[actor], map[string]string{"content": "private lifecycle reply"}, want)
	}
	reply("owner", 201)
	reply("a", 201)
	check := func(detail map[string]any, status, priority string) {
		t.Helper()
		if detail["status"] != status || detail["priority"] != priority || detail["assignee_id"] != float64(f.ids["a"]) ||
			!reflect.DeepEqual(detail, f.detail(path)) {
			t.Fatal("unexpected TicketDetail or persisted state")
		}
	}
	event := func(kind, actor, key, from, to string) {
		t.Helper()
		events := f.history(path)
		e := events[len(events)-1].(map[string]any)
		want := map[string]any{"from_" + key: from, "to_" + key: to}
		if e["event_type"] != kind || e["actor_id"] != float64(f.ids[actor]) || !reflect.DeepEqual(e["metadata"], want) {
			t.Fatal("incorrect event", kind)
		}
	}
	high := f.priority(path, "a", "high", 200)
	check(high, "in_progress", "high")
	event("priority_changed", "a", "priority", "normal", "high")
	before := f.history(path)
	if !reflect.DeepEqual(high, f.priority(path, "admin", "high", 200)) || !reflect.DeepEqual(f.detail(path), high) ||
		!reflect.DeepEqual(f.history(path), before) {
		t.Fatal("no-op priority changed ticket timestamp or history")
	}
	check(f.priority(path, "admin", "normal", 200), "in_progress", "normal")
	event("priority_changed", "admin", "priority", "high", "normal")
	check(f.post(path+"/resolve", "a", 200), "resolved", "normal")
	event("resolved", "a", "status", "in_progress", "resolved")
	reply("a", 409)
	f.post(path+"/resolve", "a", 409)
	f.priority(path, "admin", "normal", 409)
	check(f.post(path+"/reopen", "owner", 200), "in_progress", "normal")
	event("reopened", "owner", "status", "resolved", "in_progress")
	f.post(path+"/reopen", "owner", 409)
	reply("owner", 201)
	reply("a", 201)
	check(f.post(path+"/resolve", "a", 200), "resolved", "normal")
	check(f.post(path+"/close", "owner", 200), "closed", "normal")
	event("closed", "owner", "status", "resolved", "closed")
	before = f.history(path)
	f.post(path+"/close", "owner", 409)
	f.post(path+"/reopen", "owner", 409)
	f.post(path+"/resolve", "a", 409)
	f.priority(path, "a", "normal", 409)
	reply("owner", 409)
	f.call("PUT", path+"/assignee", f.tokens["admin"], map[string]uint64{"assignee_id": f.ids["b"]}, 409)
	if !reflect.DeepEqual(before, f.history(path)) {
		t.Fatal("closed ticket history changed")
	}

	path = f.processing("Admin close")
	f.call("PUT", path+"/assignee", f.tokens["admin"], map[string]uint64{"assignee_id": f.ids["b"]}, 200)
	f.post(path+"/resolve", "a", 404)
	f.priority(path, "a", "high", 404)
	f.post(path+"/resolve", "b", 200)
	f.post(path+"/close", "admin", 200)
	event("closed", "admin", "status", "resolved", "closed")
}

func testLifecycleRaces(f lifecycleTest) {
	t := f.t
	type operation struct{ action, actor string }
	reply := operation{"messages", "owner"}
	resolve := operation{"resolve", "a"}
	closeTicket := operation{"close", "admin"}
	reopen := operation{"reopen", "owner"}
	transfer := operation{"assignee", "admin"}
	for _, tc := range []struct {
		name              string
		resolved          bool
		first, second     operation
		statuses          [2]int
		finalState, owner string
		added             []string
	}{
		{"reply then resolve", false, reply, resolve, [2]int{201, 200}, "resolved", "a", []string{"message_added", "resolved"}},
		{"resolve then reply", false, resolve, reply, [2]int{200, 409}, "resolved", "a", []string{"resolved"}},
		{"reply on resolved then close", true, reply, closeTicket, [2]int{409, 200}, "closed", "a", []string{"closed"}},
		{"close then reply", true, closeTicket, reply, [2]int{200, 409}, "closed", "a", []string{"closed"}},
		{"resolve then transfer", false, resolve, transfer, [2]int{200, 409}, "resolved", "a", []string{"resolved"}},
		{"transfer then resolve", false, transfer, resolve, [2]int{200, 404}, "in_progress", "b", []string{"reassigned"}},
		{"two resolves", false, resolve, resolve, [2]int{200, 409}, "resolved", "a", []string{"resolved"}},
		{"two reopens", true, reopen, reopen, [2]int{200, 409}, "in_progress", "a", []string{"reopened"}},
		{"two closes", true, closeTicket, closeTicket, [2]int{200, 409}, "closed", "a", []string{"closed"}},
	} {
		path := f.processing(tc.name)
		if tc.resolved {
			f.post(path+"/resolve", "a", 200)
		}
		prefix := len(f.history(path))
		request := func(op operation) actionRequest {
			req := actionRequest{"POST", f.base + path + "/" + op.action, f.tokens[op.actor], ""}
			switch op.action {
			case "messages":
				req.body = `{"content":"private lifecycle reply"}`
			case "assignee":
				req.method = "PUT"
				req.body = fmt.Sprintf(`{"assignee_id":%d}`, f.ids["b"])
			}
			return req
		}
		got := orderedTicketActions(t, f.db, f.ids[tc.first.actor], request(tc.first), request(tc.second))
		detail := f.detail(path)
		if got != tc.statuses || detail["status"] != tc.finalState || detail["assignee_id"] != float64(f.ids[tc.owner]) {
			t.Fatalf("%s: unexpected result, HTTP %v", tc.name, got)
		}
		var added []string
		for _, row := range f.history(path)[prefix:] {
			added = append(added, row.(map[string]any)["event_type"].(string))
		}
		messages := f.call("GET", path+"/messages", f.tokens["owner"], nil, 200)
		wantMessages := 0.0
		if tc.first == reply && got[0] == 201 {
			wantMessages = 1
		}
		if !reflect.DeepEqual(added, tc.added) || messages["total"] != wantMessages {
			t.Fatal(tc.name, "incorrect events or messages")
		}
		t.Logf("%s: HTTP %v, events %v", tc.name, got, added)
	}
}

func testLifecycleRollback(f lifecycleTest) {
	t := f.t
	for _, target := range []string{"update", "event"} {
		path := f.processing("block " + target)
		before := f.detail(path)
		events := f.history(path)
		// The test schema is empty on entry. Restrict injected failures to this ticket.
		stmt := "ALTER TABLE tickets ADD CONSTRAINT test_block_lifecycle_update CHECK (title <> 'block update' OR (status = 'in_progress' AND priority = 'normal'))"
		if target == "event" {
			stmt = fmt.Sprintf("ALTER TABLE ticket_events ADD CONSTRAINT test_block_lifecycle_event CHECK (ticket_id <> %.0f OR event_type NOT IN ('priority_changed','resolved'))", before["id"])
		}
		if err := f.db.Exec(stmt).Error; err != nil {
			t.Fatal("cannot install test constraint")
		}
		for _, action := range []string{"priority", "resolve"} {
			var res map[string]any
			if action == "priority" {
				res = f.priority(path, "admin", "high", 500)
			} else {
				res = f.post(path+"/resolve", "a", 500)
			}
			if res["code"] != "internal_error" || res["message"] != "internal server error" ||
				!reflect.DeepEqual(f.detail(path), before) || !reflect.DeepEqual(f.history(path), events) {
				t.Fatal("partial write or unsafe error", target, action)
			}
		}
	}
}
