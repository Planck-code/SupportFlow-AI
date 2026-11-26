//go:build integration

package integration

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Planck-code/SupportFlow-AI/internal/ai"
	"github.com/Planck-code/SupportFlow-AI/internal/model"
	"github.com/Planck-code/SupportFlow-AI/internal/repository"
)

type runReply struct {
	status   int
	body     map[string]any
	location string
	err      error
}

func postRun(ctx context.Context, url, token string) runReply {
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return runReply{err: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return runReply{err: err}
	}
	defer resp.Body.Close()
	r := runReply{status: resp.StatusCode, location: resp.Header.Get("Location")}
	r.err = json.NewDecoder(resp.Body).Decode(&r.body)
	return r
}

func (r runReply) check(t *testing.T, status int) {
	t.Helper()
	if r.err != nil || r.status != status {
		t.Fatalf("analysis HTTP %d, want %d (transport error %v)", r.status, status, r.err)
	}
	requestID, _ := r.body["request_id"].(string)
	if status >= 400 && (len(r.body) != 3 || r.body["code"] == nil || r.body["message"] == nil || requestID == "") {
		t.Fatal("unsafe error envelope")
	}
}

func TestAIRunsLiveMySQL(t *testing.T) {
	db, runner := testDatabase(t)
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, runner, latestVersion)
	var mode atomic.Value
	mode.Store("success")
	entered := make(chan struct{}, 1)
	release := make(chan struct{}, 1)
	const summary = "local mock private summary"
	key := rand.Text()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var call struct {
			Messages []ai.Message
			Model    string
		}
		if json.NewDecoder(r.Body).Decode(&call) != nil || r.URL.Path != "/v1/chat/completions" ||
			r.Header.Get("Authorization") != "Bearer "+key || call.Model != "fixture" ||
			len(call.Messages) != 2 || call.Messages[0].Role != "system" || call.Messages[1].Role != "user" ||
			!strings.Contains(call.Messages[1].Content, "private lifecycle description") {
			t.Error("unexpected model request")
			return
		}
		content := `{"category":"billing","summary":"local mock private summary","reply_suggestion":"draft only"}`
		switch mode.Load().(string) {
		case "block":
			entered <- struct{}{}
			select {
			case <-r.Context().Done():
				return
			case <-release:
			}
		case "timeout":
			<-r.Context().Done()
			return
		case "upstream":
			http.Error(w, "private provider error", 429)
			return
		case "invalid_response":
			fmt.Fprint(w, "{}")
			return
		case "invalid_output":
			content = `{"category":"account","summary":"x","summary":"y","reply_suggestion":"z"}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": ai.Message{Role: "assistant", Content: content}}}})
	}))
	defer upstream.Close()
	base, logs := startTestAPI(t, os.Getenv("TEST_DATABASE_NAME"), "AI_BASE_URL="+upstream.URL+"/v1", "AI_API_KEY="+key, "AI_MODEL=fixture", "AI_TIMEOUT=3s", "DB_MAX_OPEN_CONNS=1", "DB_MAX_IDLE_CONNS=1")
	base += "/api/v1"
	ids, tokens, pass := seedTicketActors(t, db, base)
	f := lifecycleTest{t, db, base, ids, tokens, liveRequests(t, base)}
	path := f.processing("AI")
	runs := path + "/ai-runs"
	trigger := func(actor string) runReply { return postRun(context.Background(), base+runs, tokens[actor]) }
	get := func(url, actor string, want int) map[string]any { return f.call("GET", url, tokens[actor], nil, want) }
	for actor, status := range map[string]int{"": 401, "owner": 403, "admin": 403, "b": 404} {
		trigger(actor).check(t, status)
	}
	f.call("POST", runs, tokens["a"], map[string]string{"prompt": "override"}, 400)
	before := f.detail(path)
	r := trigger("a")
	r.check(t, 201)
	runPath := fmt.Sprintf("%s/%.0f", runs, r.body["id"])
	if r.location != "/api/v1"+runPath || !reflect.DeepEqual(r.body, get(runPath, "admin", 200)) || !reflect.DeepEqual(r.body, get(runPath, "a", 200)) {
		t.Fatal("created run not retrievable")
	}
	result := r.body["result"].(map[string]any)
	if result["category"] != "billing" || result["summary"] != summary || len(result) != 3 || r.body["status"] != "succeeded" || r.body["model"] != "fixture" || r.body["finished_at"] == nil || r.body["created_at"] == nil || r.body["error_category"] != nil || r.body["input_truncated"] != false || !strings.HasPrefix(r.body["input_snapshot_id"].(string), "v1:") {
		t.Fatal("invalid success record")
	}
	if err := repository.NewTickets(db).FinishRun(context.Background(), uint64(r.body["id"].(float64)), nil, "upstream"); err == nil || !reflect.DeepEqual(r.body, get(runPath, "admin", 200)) {
		t.Fatal("terminal state was overwritten")
	}
	get(runPath, "owner", 403)
	get(runPath, "b", 404)
	get(f.processing("other")+"/ai-runs/"+fmt.Sprintf("%.0f", r.body["id"]), "admin", 404)

	for _, tc := range []struct {
		category string
		status   int
	}{{"upstream", 502}, {"invalid_response", 502}, {"invalid_output", 502}, {"timeout", 504}} {
		mode.Store(tc.category)
		reply := trigger("a")
		reply.check(t, tc.status)
		if !strings.HasPrefix(reply.location, "/api/v1"+runs+"/") {
			t.Fatal("failed run missing safe reference")
		}
		record := get(strings.TrimPrefix(reply.location, "/api/v1"), "a", 200)
		if record["status"] != "failed" || record["error_category"] != tc.category || record["result"] != nil || record["finished_at"] == nil {
			t.Fatal("failed run not persisted")
		}
	}
	if !reflect.DeepEqual(before, f.detail(path)) || get(path+"/messages", "a", 200)["total"] != float64(0) {
		t.Fatal("analysis changed business data")
	}

	blocked := func(ctx context.Context, actor string) chan runReply {
		t.Helper()
		mode.Store("block")
		done := make(chan runReply, 1)
		go func() { done <- postRun(ctx, base+runs, tokens[actor]) }()
		select {
		case <-entered:
		case <-time.After(4 * time.Second):
			t.Fatal("provider not reached")
		}
		return done
	}
	pending := func() model.AIRun {
		t.Helper()
		var row model.AIRun
		if err := db.Where("status = 'running'").Take(&row).Error; err != nil {
			t.Fatal("running row not committed")
		}
		return row
	}
	// An API pool of one connection makes a retained connection observable too.
	done := blocked(context.Background(), "a")
	row := pending()
	active := runs + "/" + strconv.FormatUint(row.ID, 10)
	if record := get(active, "a", 200); record["result"] != nil || record["finished_at"] != nil || record["status"] != "running" {
		t.Fatal("bad pending state")
	}
	f.call("POST", path+"/messages", tokens["owner"], map[string]string{"content": "new message"}, 201)
	f.call("PUT", path+"/assignee", tokens["admin"], map[string]uint64{"assignee_id": ids["b"]}, 200)
	release <- struct{}{}
	hidden := <-done
	hidden.check(t, 404)
	if hidden.location != "" {
		t.Fatal("run location disclosed after transfer")
	}
	get(active, "a", 404)
	trigger("a").check(t, 404)
	if get(active, "b", 200)["status"] != "succeeded" {
		t.Fatal("authorized run not finalized")
	}

	done = blocked(context.Background(), "b")
	f.post(path+"/resolve", "b", 200)
	release <- struct{}{}
	(<-done).check(t, 201)
	trigger("b").check(t, 409)
	get(active, "admin", 200)
	f.post(path+"/reopen", "owner", 200)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done = blocked(ctx, "b")
	row = pending()
	cancel()
	<-done
	deadline := time.Now().Add(4 * time.Second)
	for {
		var final model.AIRun
		if err := db.Take(&final, row.ID).Error; err != nil {
			t.Fatal(err)
		}
		if final.Status == "failed" && final.FinishedAt != nil && final.ErrorCategory != nil && *final.ErrorCategory == "canceled" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("disconnect left running record")
		}
		time.Sleep(15 * time.Millisecond)
	}

	// Apply only to future rows in this verified empty, test-owned schema.
	var last uint64
	if db.Raw("SELECT MAX(id) FROM ai_runs").Scan(&last).Error != nil {
		t.Fatal("cannot inspect run ids")
	}
	failurePath := f.processing("terminal write failure")
	ticketID, err := strconv.ParseUint(strings.TrimPrefix(failurePath, "/tickets/"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	constraint := fmt.Sprintf("ALTER TABLE ai_runs ADD CONSTRAINT private_storage_error CHECK (ticket_id <> %d OR status <> 'succeeded')", ticketID)
	if err := db.Exec(constraint).Error; err != nil {
		t.Fatal("cannot install failure fixture")
	}
	mode.Store("success")
	dbFailure := postRun(context.Background(), base+failurePath+"/ai-runs", tokens["a"])
	dbFailure.check(t, 500)
	if dbFailure.location != "" || dbFailure.body["message"] != "internal server error" || pending().ID <= last {
		t.Fatal("storage failure reported as terminal")
	}
	disabled, _ := startTestAPI(t, os.Getenv("TEST_DATABASE_NAME"), "AI_TIMEOUT=invalid")
	disabledCall := liveRequests(t, disabled+"/api/v1")
	login := disabledCall("POST", "/auth/login", "", map[string]string{"email": "b@example.test", "password": pass}, 200)["access_token"].(string)
	value := disabledCall("GET", active, login, nil, 200)
	if value["status"] != "succeeded" {
		t.Fatal("AI history needs provider config")
	}
	disabledCall("POST", runs, login, map[string]any{}, 503)
	for _, path := range []string{"/healthz", "/readyz"} {
		liveRequests(t, disabled)("GET", path, "", nil, 200)
	}
	log, err := os.ReadFile(logs)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{key, summary, pass, tokens["a"], "private lifecycle description", "private_storage_error", "private provider error"} {
		if strings.Contains(string(log), secret) {
			t.Fatal("sensitive data logged")
		}
	}
}
