//go:build integration

package integration

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/Planck-code/SupportFlow-AI/internal/model"
	"github.com/Planck-code/SupportFlow-AI/internal/repository"
	"github.com/Planck-code/SupportFlow-AI/internal/service"
)

// The first action holds the ticket while it waits for its actor's shared role
// lock. Only release the actor after observing the second action at the ticket lock.
func orderedTicketActions(t *testing.T, db *gorm.DB, firstActor uint64, first, second actionRequest) [2]int {
	t.Helper()
	guard := db.Begin()
	if guard.Error != nil {
		t.Fatal("failed to begin actor lock")
	}
	defer guard.Rollback()
	var locked uint64
	if err := guard.Raw("SELECT id FROM users WHERE id = ? FOR UPDATE", firstActor).Scan(&locked).Error; err != nil || locked != firstActor {
		t.Fatal("failed to lock test actor")
	}
	client := &http.Client{Timeout: 8 * time.Second}
	start := func(a actionRequest) <-chan actionResponse {
		out := make(chan actionResponse, 1)
		go func() { out <- sendAction(client, a) }()
		return out
	}
	waitFor := func(pattern string) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			var n int
			err := db.Raw("SELECT COUNT(*) FROM information_schema.processlist WHERE DB = DATABASE() AND ID <> CONNECTION_ID() AND LOWER(INFO) LIKE ?", pattern).Scan(&n).Error
			if err != nil {
				t.Fatal("cannot inspect test lock waiters")
			}
			if n > 0 {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("expected a waiting request at %s", pattern)
	}
	a := start(first)
	waitFor("select%from %users%for share%")
	b := start(second)
	waitFor("select%from %tickets%for update%")
	if guard.Rollback().Error != nil {
		t.Fatal("cannot release test actor")
	}
	one, two := <-a, <-b
	if one.err != nil || two.err != nil {
		t.Fatal("HTTP race failed")
	}
	return [2]int{one.status, two.status}
}

func TestMessagesLiveMySQL(t *testing.T) {
	db, runner := testDatabase(t)
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, runner, latestVersion)
	base, logPath := startTestAPI(t, os.Getenv("TEST_DATABASE_NAME"))
	base += "/api/v1"
	call := liveRequests(t, base)
	reg := service.NewRegistration(repository.NewUsers(db))
	ids := map[string]uint64{}
	tokens := map[string]string{}
	password := rand.Text()
	for name, role := range map[string]string{"owner": "user", "other": "user", "a": "agent", "b": "agent", "admin": "admin"} {
		email := name + "@example.test"
		var u model.User
		var err error
		if role == model.RoleUser {
			u, err = reg.Register(context.Background(), email, password, name)
		} else {
			u, err = reg.CreateStaff(context.Background(), email, password, name, role)
		}
		if err != nil {
			t.Fatal("cannot create test actor")
		}
		ids[name] = u.ID
		tokens[name] = call("POST", "/auth/login", "", map[string]string{"email": email, "password": password}, 200)["access_token"].(string)
	}
	create := func() (uint64, string) {
		id := uint64(call("POST", "/tickets", tokens["owner"], map[string]string{"title": "Messages", "description": "Conversation"}, 201)["id"].(float64))
		return id, fmt.Sprintf("/tickets/%d", id)
	}
	const privateText = "  private conversation 📨  "
	reply := func(path, actor, content string, status int) map[string]any {
		t.Helper()
		return call("POST", path+"/messages", tokens[actor], map[string]string{"content": content}, status)
	}
	count := func(table string, id uint64) int64 {
		t.Helper()
		var n int64
		if err := db.Table(table).Where("ticket_id = ?", id).Count(&n).Error; err != nil {
			t.Fatal("failed to count test rows")
		}
		return n
	}
	id, path := create()
	call("GET", path+"/messages", "", nil, 401)
	reply(path, "", privateText, 401)
	for _, who := range []string{"a", "other"} {
		call("GET", path+"/messages", tokens[who], nil, 404)
		reply(path, who, privateText, 404)
	}
	reply(path, "admin", privateText, 403)
	reply("/tickets/0", "owner", privateText, 400)
	reply("/tickets/987654", "owner", privateText, 404)
	call("GET", "/tickets/987654/messages", tokens["admin"], nil, 404)
	call("GET", "/tickets/bad/messages", tokens["owner"], nil, 400)
	call("GET", path+"/messages?page_size=101", tokens["owner"], nil, 400)
	empty := call("GET", path+"/messages", tokens["admin"], nil, 200)
	if len(empty["items"].([]any)) != 0 || empty["page"] != 1.0 || empty["page_size"] != 20.0 || empty["total"] != 0.0 {
		t.Fatal("invalid empty message page")
	}
	for _, bad := range []string{"", " \t\n", strings.Repeat("界", 5001)} {
		reply(path, "owner", bad, 400)
	}
	call("POST", path+"/messages", tokens["owner"], map[string]any{"content": privateText, "author_id": ids["admin"]}, 400)
	boundary := strings.Repeat("📨", 5000)
	messages := []map[string]any{reply(path, "owner", boundary, 201)}
	call("POST", path+"/claim", tokens["a"], map[string]any{}, 200)
	messages = append(messages, reply(path, "owner", privateText, 201), reply(path, "a", "Acknowledged", 201))
	reply(path, "b", privateText, 404)
	page := call("GET", path+"/messages?page=2&page_size=2", tokens["a"], nil, 200)
	if page["total"] != 3.0 || page["page"] != 2.0 || page["page_size"] != 2.0 || !reflect.DeepEqual(page["items"], []any{messages[2]}) {
		t.Fatal("wrong second page")
	}
	call("PUT", path+"/assignee", tokens["admin"], map[string]uint64{"assignee_id": ids["b"]}, 200)
	call("GET", path+"/messages", tokens["a"], nil, 404)
	reply(path, "a", privateText, 404)
	messages = append(messages, reply(path, "b", "Taking over", 201))
	for _, who := range []string{"b", "admin", "owner"} {
		rows := call("GET", path+"/messages", tokens[who], nil, 200)["items"].([]any)
		if len(rows) != len(messages) {
			t.Fatal("unexpected message count")
		}
		for i, got := range rows {
			if !reflect.DeepEqual(got, messages[i]) {
				t.Fatal("listed message differs from POST or ordering is wrong")
			}
		}
	}
	var stored []model.TicketMessage
	var events []model.TicketEvent
	if err := db.Where("ticket_id = ?", id).Order("id ASC").Find(&stored).Error; err != nil || len(stored) != len(messages) {
		t.Fatal("incorrect database messages")
	}
	if err := db.Where("ticket_id = ? AND event_type = 'message_added'", id).Order("id ASC").Find(&events).Error; err != nil || len(events) != len(stored) {
		t.Fatal("incorrect database events")
	}
	authors := []string{"owner", "owner", "a", "b"}
	contents := []string{boundary, privateText, "Acknowledged", "Taking over"}
	for i, msg := range stored {
		m := messages[i]
		if len(m) != 5 || m["id"] != float64(msg.ID) || m["content"] != contents[i] || msg.Content != contents[i] ||
			m["ticket_id"] != float64(id) || m["author_id"] != float64(ids[authors[i]]) || msg.AuthorID != ids[authors[i]] ||
			m["created_at"] != msg.CreatedAt.UTC().Format(time.RFC3339Nano) {
			t.Fatal("unsafe or incorrect message response")
		}
		var meta map[string]uint64
		if json.Unmarshal(events[i].Metadata, &meta) != nil || len(meta) != 1 || meta["message_id"] != msg.ID || events[i].ActorID != msg.AuthorID {
			t.Fatal("incorrect message event metadata or author")
		}
	}

	for _, state := range []string{"resolved", "closed"} {
		if err := db.Model(&model.Ticket{}).Where("id = ?", id).Update("status", state).Error; err != nil {
			t.Fatal(err)
		}
		reply(path, "owner", privateText, 409)
		reply(path, "b", privateText, 409)
		reply(path, "other", privateText, 404)
		call("GET", path+"/messages", tokens["owner"], nil, 200)
	}
	for _, replyFirst := range []bool{true, false} {
		raceID, racePath := create()
		call("POST", racePath+"/claim", tokens["a"], map[string]any{}, 200)
		post := actionRequest{"POST", base + racePath + "/messages", tokens["a"], `{"content":"race reply"}`}
		put := actionRequest{"PUT", base + racePath + "/assignee", tokens["admin"], fmt.Sprintf(`{"assignee_id":%d}`, ids["b"])}
		wantTypes := []string{"created", "claimed"}
		var got [2]int
		var wantMessages int64
		if replyFirst {
			got = orderedTicketActions(t, db, ids["a"], post, put)
			if got != [2]int{201, 200} {
				t.Fatal("reply-first HTTP statuses", got)
			}
			wantTypes = append(wantTypes, "message_added", "reassigned")
			wantMessages = 1
		} else {
			got = orderedTicketActions(t, db, ids["admin"], put, post)
			if got != [2]int{200, 404} {
				t.Fatal("transfer-first HTTP statuses", got)
			}
			wantTypes = append(wantTypes, "reassigned")
		}
		var types []string
		if err := db.Model(&model.TicketEvent{}).Where("ticket_id = ?", raceID).Order("id ASC").Pluck("event_type", &types).Error; err != nil ||
			!reflect.DeepEqual(types, wantTypes) || count("ticket_messages", raceID) != wantMessages {
			t.Fatal("event order or message count contradicts lock order")
		}
		t.Logf("replyFirst=%v: HTTP %v, database events %v", replyFirst, got, types)
	}

	for _, table := range []string{"ticket_events", "ticket_messages"} {
		testID, testPath := create()
		predicate := fmt.Sprintf("ticket_id <> %d", testID)
		if table == "ticket_events" {
			predicate += " OR event_type <> 'message_added'"
		}
		if err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT test_refuse_%s CHECK (%s)", table, table, predicate)).Error; err != nil {
			t.Fatal("cannot install test constraint")
		}
		res := reply(testPath, "owner", privateText, 500)
		if res["message"] != "internal server error" || res["code"] != "internal_error" || count("ticket_messages", testID) != 0 || count("ticket_events", testID) != 1 {
			t.Fatal("insert failure disclosed details or left partial changes")
		}
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	hidden := []string{privateText, boundary, password, os.Getenv("DB_PASSWORD"), "test_refuse_", "INSERT INTO"}
	for _, tok := range tokens {
		hidden = append(hidden, tok)
	}
	for _, value := range hidden {
		if value != "" && strings.Contains(string(raw), value) {
			t.Fatal("sensitive data in API log")
		}
	}
}
