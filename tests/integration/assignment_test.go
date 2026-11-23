//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/Planck-code/SupportFlow-AI/internal/model"
	"github.com/Planck-code/SupportFlow-AI/internal/repository"
	"github.com/Planck-code/SupportFlow-AI/internal/service"
)

type actionRequest struct{ method, url, token, body string }
type actionResponse struct {
	status int
	err    error
}

func sendAction(client *http.Client, a actionRequest) actionResponse {
	req, err := http.NewRequest(a.method, a.url, strings.NewReader(a.body))
	if err != nil {
		return actionResponse{err: err}
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return actionResponse{err: err}
	}
	defer resp.Body.Close()
	_, err = io.Copy(io.Discard, resp.Body)
	return actionResponse{status: resp.StatusCode, err: err}
}

// Hold the test row until both independent HTTP transactions actually reach
// SELECT FOR UPDATE. This tests DB contention independently of OS scheduling.
func raceActions(t *testing.T, db *gorm.DB, id uint64, pair [2]actionRequest) [2]int {
	t.Helper()
	guard := db.Begin()
	if guard.Error != nil {
		t.Fatal("cannot begin test lock")
	}
	defer guard.Rollback()
	var locked uint64
	if err := guard.Raw("SELECT id FROM tickets WHERE id = ? FOR UPDATE", id).Scan(&locked).Error; err != nil || locked != id {
		t.Fatal("test row lock failed")
	}
	var replies [2]chan actionResponse
	client := &http.Client{Timeout: 8 * time.Second}
	for i, a := range pair {
		replies[i] = make(chan actionResponse, 1)
		go func(out chan<- actionResponse, a actionRequest) { out <- sendAction(client, a) }(replies[i], a)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		var waiters int64
		err := db.Raw("SELECT COUNT(*) FROM information_schema.processlist WHERE ID <> CONNECTION_ID() AND DB = DATABASE() AND LOWER(INFO) LIKE 'select%from %tickets%for update%'").Scan(&waiters).Error
		if err != nil {
			t.Fatal("cannot observe test sessions")
		}
		if waiters >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("two HTTP transactions did not reach the ticket lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if guard.Rollback().Error != nil {
		t.Fatal("cannot release test row")
	}
	var statuses [2]int
	for i, out := range replies {
		res := <-out
		if res.err != nil {
			t.Fatal("network request failed")
		}
		statuses[i] = res.status
	}
	return statuses
}

func TestAssignmentLiveMySQL(t *testing.T) {
	db, runner := testDatabase(t)
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, runner, latestVersion)
	base, logs := startTestAPI(t, os.Getenv("TEST_DATABASE_NAME"))
	call := liveRequests(t, base+"/api/v1")
	users := repository.NewUsers(db)
	reg := service.NewRegistration(users)
	ids := map[string]uint64{}
	tokens := map[string]string{}
	secret := rand.Text()
	for name, role := range map[string]string{"owner": "user", "a": "agent", "b": "agent", "admin": "admin"} {
		var u model.User
		var err error
		email := name + "@example.test"
		if role == "user" {
			u, err = reg.Register(context.Background(), email, secret, name)
		} else {
			u, err = reg.CreateStaff(context.Background(), email, secret, name, role)
		}
		if err != nil {
			t.Fatal("cannot seed test identity")
		}
		ids[name] = u.ID
		result := call("POST", "/auth/login", "", map[string]string{"email": email, "password": secret}, 200)
		tokens[name] = result["access_token"].(string)
	}
	const description = "private-assignment-test-text"
	create := func() uint64 {
		t.Helper()
		m := call("POST", "/tickets", tokens["owner"], map[string]string{"title": "Assign", "description": description}, 201)
		return uint64(m["id"].(float64))
	}
	path := func(id uint64) string { return fmt.Sprintf("/tickets/%d", id) }
	assign := func(id uint64, who string, target uint64, status int) map[string]any {
		t.Helper()
		return call("PUT", path(id)+"/assignee", tokens[who], map[string]uint64{"assignee_id": target}, status)
	}
	claim := func(id uint64, who string, status int) map[string]any {
		t.Helper()
		return call("POST", path(id)+"/claim", tokens[who], map[string]any{}, status)
	}
	events := func(id uint64) []any {
		t.Helper()
		return call("GET", path(id)+"/events", tokens["owner"], nil, 200)["items"].([]any)
	}
	detail := func(id uint64) map[string]any { t.Helper(); return call("GET", path(id), tokens["owner"], nil, 200) }
	assertOwner := func(m map[string]any, agent string) {
		t.Helper()
		if m["status"] != "in_progress" || m["assignee_id"] != float64(ids[agent]) || m["description"] != description {
			t.Fatal("incorrect assignment detail")
		}
	}
	assertEvent := func(raw any, kind, actor, from, to string) {
		t.Helper()
		e := raw.(map[string]any)
		meta := e["metadata"].(map[string]any)
		var prev any
		oldState := "open"
		if from != "" {
			prev = float64(ids[from])
			oldState = "in_progress"
		}
		if e["event_type"] != kind || e["actor_id"] != float64(ids[actor]) || len(meta) != 4 || meta["from_status"] != oldState || meta["to_status"] != "in_progress" || meta["from_assignee_id"] != prev || meta["to_assignee_id"] != float64(ids[to]) {
			t.Fatal("incorrect event metadata or actor")
		}
	}

	direct := create()
	assertOwner(claim(direct, "a", 200), "a")
	claim(direct, "a", 409)
	assertEvent(events(direct)[1], "claimed", "a", "", "a")

	id := create()
	claim(id, "", 401)
	assign(id, "", ids["a"], 401)
	claim(id, "owner", 403)
	claim(id, "admin", 403)
	assign(id, "a", ids["a"], 403)
	assign(id, "owner", ids["a"], 403)
	claim(99999, "a", 404)
	assign(99999, "admin", ids["a"], 404)
	call("POST", "/tickets/0/claim", tokens["a"], map[string]any{}, 400)
	call("PUT", "/tickets/x/assignee", tokens["admin"], map[string]uint64{"assignee_id": ids["a"]}, 400)
	call("POST", path(id)+"/claim", tokens["a"], map[string]any{"actor_id": ids["b"]}, 400)
	call("PUT", path(id)+"/assignee", tokens["admin"], map[string]any{"assignee_id": ids["a"], "status": "closed"}, 400)
	for _, bad := range []uint64{0, ids["owner"], ids["admin"], 999999} {
		assign(id, "admin", bad, 400)
	}
	first := assign(id, "admin", ids["a"], 200)
	assertOwner(first, "a")
	same := assign(id, "admin", ids["a"], 200)
	if same["updated_at"] != first["updated_at"] || detail(id)["updated_at"] != first["updated_at"] || len(events(id)) != 2 {
		t.Fatal("idempotent assignment changed state/history")
	}
	second := assign(id, "admin", ids["b"], 200)
	assertOwner(second, "b")
	history := events(id)
	if len(history) != 3 {
		t.Fatal("wrong assignment history length")
	}
	assertEvent(history[1], "assigned", "admin", "", "a")
	assertEvent(history[2], "reassigned", "admin", "a", "b")
	for _, suffix := range []string{"", "/events"} {
		call("GET", path(id)+suffix, tokens["a"], nil, 404)
		for _, who := range []string{"b", "owner", "admin"} {
			call("GET", path(id)+suffix, tokens[who], nil, 200)
		}
	}
	claim(id, "a", 404)
	claim(id, "b", 409)
	for _, state := range []string{"resolved", "closed"} {
		if err := db.Model(&model.Ticket{}).Where("id = ?", id).Update("status", state).Error; err != nil {
			t.Fatal(err)
		}
		assign(id, "admin", ids["b"], 409)
		claim(id, "b", 409)
		claim(id, "a", 404)
	}

	contested := create()
	claimRequest := func(id uint64, agent string) actionRequest {
		return actionRequest{"POST", base + "/api/v1" + path(id) + "/claim", tokens[agent], ""}
	}
	statuses := raceActions(t, db, contested, [2]actionRequest{claimRequest(contested, "a"), claimRequest(contested, "b")})
	winner := "a"
	switch statuses {
	case [2]int{200, 409}:
	case [2]int{409, 200}:
		winner = "b"
	default:
		t.Fatal("claim race", statuses)
	}
	assertOwner(detail(contested), winner)
	assertOwner(call("GET", path(contested), tokens[winner], nil, 200), winner)
	history = events(contested)
	if len(history) != 2 {
		t.Fatal("duplicate claim event")
	}
	assertEvent(history[1], "claimed", winner, "", winner)
	call("GET", path(contested)+"/events", tokens[winner], nil, 200)
	t.Logf("two queued DB lock requests: claim statuses %v; one claimed event", statuses)

	mixed := create()
	statuses = raceActions(t, db, mixed, [2]actionRequest{claimRequest(mixed, "a"), {"PUT", base + "/api/v1" + path(mixed) + "/assignee", tokens["admin"], fmt.Sprintf(`{"assignee_id":%d}`, ids["b"])}})
	history = events(mixed)
	assertOwner(detail(mixed), "b")
	switch statuses {
	case [2]int{200, 200}:
		if len(history) != 3 {
			t.Fatal("missing transfer history")
		}
		assertEvent(history[1], "claimed", "a", "", "a")
		assertEvent(history[2], "reassigned", "admin", "a", "b")
	case [2]int{409, 200}:
		if len(history) != 2 {
			t.Fatal("unexpected mixed events")
		}
		assertEvent(history[1], "assigned", "admin", "", "b")
	default:
		t.Fatal("invalid mixed serialization", statuses)
	}
	t.Logf("claim vs admin statuses %v; validated event sequence and final owner", statuses)

	rollback := create()
	transfer := create()
	beforeTransfer := assign(transfer, "admin", ids["a"], 200)
	constraint := fmt.Sprintf("ALTER TABLE ticket_events ADD CONSTRAINT test_block_assignment CHECK ((ticket_id <> %d OR event_type = 'created') AND (ticket_id <> %d OR event_type <> 'reassigned'))", rollback, transfer)
	if err := db.Exec(constraint).Error; err != nil {
		t.Fatal("cannot install isolated test constraint")
	}
	original := detail(rollback)
	for _, failure := range []map[string]any{claim(rollback, "a", 500), assign(rollback, "admin", ids["b"], 500)} {
		if failure["code"] != "internal_error" || failure["message"] != "internal server error" {
			t.Fatal("internal failure leaked")
		}
	}
	after := detail(rollback)
	if after["assignee_id"] != nil || after["status"] != "open" || after["updated_at"] != original["updated_at"] || len(events(rollback)) != 1 {
		t.Fatal("failed event left a partial update")
	}
	assign(transfer, "admin", ids["b"], 500)
	afterTransfer := detail(transfer)
	assertOwner(afterTransfer, "a")
	if afterTransfer["updated_at"] != beforeTransfer["updated_at"] || len(events(transfer)) != 2 {
		t.Fatal("failed transfer changed the ticket or history")
	}
	raw, err := os.ReadFile(logs)
	if err != nil {
		t.Fatal(err)
	}
	sensitive := []string{description, secret, os.Getenv("DB_PASSWORD"), "test_block_assignment", "INSERT INTO"}
	for _, token := range tokens {
		sensitive = append(sensitive, token)
	}
	for _, value := range sensitive {
		if value != "" && bytes.Contains(raw, []byte(value)) {
			t.Fatal("sensitive API logs")
		}
	}
}
