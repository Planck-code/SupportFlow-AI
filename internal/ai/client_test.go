package ai_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Planck-code/SupportFlow-AI/internal/ai"
	"github.com/Planck-code/SupportFlow-AI/internal/ai/aitest"
	"github.com/Planck-code/SupportFlow-AI/internal/config"
)

func client(t *testing.T, base, key string, timeout time.Duration) ai.Client {
	t.Helper()
	c, err := ai.NewClient(config.AI{BaseURL: base, APIKey: key, Model: "test-model", Timeout: timeout})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestChatProtocol(t *testing.T) {
	key := rand.Text()
	messages := []ai.Message{{Role: "system", Content: "summarize"}, {Role: "user", Content: "工单内容"}}
	content := `{"category":"account","summary":"需要帮助","reply_suggestion":"请说明情况"}`
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.RequestURI() != "/v1/chat/completions" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Authorization") != "Bearer "+key {
			t.Error("incorrect request method, URL or headers")
		}
		var in struct {
			Model    string
			Messages []ai.Message
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Model != "test-model" || !reflect.DeepEqual(in.Messages, messages) {
			t.Error("incorrect request payload")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": ai.Message{Role: "assistant", Content: content}}}})
	}))
	defer s.Close()
	raw, err := client(t, s.URL+"/v1/", key, time.Second).Complete(context.Background(), messages)
	if err != nil || raw != content {
		t.Fatal("failed to extract assistant content")
	}
	result, err := ai.ParseSuggestion(raw)
	if err != nil || result != (ai.Suggestion{Category: "account", Summary: "需要帮助", ReplySuggestion: "请说明情况"}) {
		t.Fatal("incorrect suggestion")
	}
}

func TestErrorBoundaries(t *testing.T) {
	key, prompt, rawError := rand.Text(), rand.Text(), rand.Text()
	var logs bytes.Buffer
	writer := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(writer)
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"unauthorized", 401, rawError, nil}, {"rate limit", 429, rawError, nil}, {"server error", 500, rawError, nil},
		{"malformed", 200, rawError, ai.ErrResponse}, {"empty response", 204, "", ai.ErrResponse},
		{"missing choices", 200, "{}", ai.ErrResponse}, {"missing message", 200, `{"choices":[{}]}`, ai.ErrResponse},
		{"empty content", 200, `{"choices":[{"message":{"content":" \t "}}]}`, ai.ErrOutput},
		{"missing content", 200, `{"choices":[{"message":{}}]}`, ai.ErrOutput},
		{"not text", 200, `{"choices":[{"message":{"content":[1]}}]}`, ai.ErrOutput},
		{"oversized valid JSON", 200, `{"choices":[{"message":{"content":"ok"}}]}` + strings.Repeat(" ", 1<<20), ai.ErrResponse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer s.Close()
			got, err := client(t, s.URL, key, time.Second).Complete(context.Background(), []ai.Message{{Role: "user", Content: prompt}})
			if err == nil || got != "" || calls.Load() != 1 {
				t.Fatal("expected a single failed attempt")
			}
			if tc.want != nil {
				if !errors.Is(err, tc.want) {
					t.Fatalf("unexpected category %T", err)
				}
			} else {
				var status *ai.UpstreamError
				if !errors.As(err, &status) || status.StatusCode != tc.status {
					t.Fatal("upstream status lost")
				}
			}
			for _, sensitive := range []string{key, prompt, rawError, "Bearer ", "Authorization"} {
				if strings.Contains(err.Error(), sensitive) || strings.Contains(logs.String(), sensitive) {
					t.Fatal("sensitive diagnostics")
				}
			}
		})
	}
}

func TestDeadlinesAndCancellation(t *testing.T) {
	for _, which := range []string{"client deadline", "earlier caller deadline", "active cancel"} {
		t.Run(which, func(t *testing.T) {
			reading := make(chan struct{})
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.WriteHeader(200)
				w.(http.Flusher).Flush()
				close(reading)
				<-r.Context().Done()
			}))
			defer s.Close()
			timeout := 5 * time.Second
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch which {
			case "client deadline":
				timeout = 150 * time.Millisecond
			case "earlier caller deadline":
				var stop context.CancelFunc
				ctx, stop = context.WithTimeout(ctx, 150*time.Millisecond)
				defer stop()
			}
			c := client(t, s.URL, rand.Text(), timeout)
			done := make(chan error, 1)
			go func() { _, err := c.Complete(ctx, []ai.Message{{Role: "user", Content: "test"}}); done <- err }()
			if which == "active cancel" {
				select {
				case <-reading:
					cancel()
				case <-time.After(2 * time.Second):
					t.Fatal("request did not arrive")
				}
			}
			want := context.DeadlineExceeded
			if which == "active cancel" {
				want = context.Canceled
			}
			select {
			case err := <-done:
				if !errors.Is(err, want) {
					t.Fatal("incorrect context failure")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("caller deadline/cancellation ignored")
			}
		})
	}
}

func TestNoRedirect(t *testing.T) {
	var forwarded atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { forwarded.Add(1) }))
	defer target.Close()
	for _, code := range []int{302, 307, 308} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, code) }))
			defer s.Close()
			_, err := client(t, s.URL, rand.Text(), time.Second).Complete(context.Background(), []ai.Message{{Role: "user", Content: "test"}})
			var upstream *ai.UpstreamError
			if !errors.As(err, &upstream) || upstream.StatusCode != code || forwarded.Load() != 0 {
				t.Fatal("unexpected redirect behavior")
			}
		})
	}
}

func TestNetworkAndInputFailures(t *testing.T) {
	s := httptest.NewServer(http.NotFoundHandler())
	s.Close()
	c := client(t, s.URL, rand.Text(), time.Second)
	if _, err := c.Complete(context.Background(), []ai.Message{{Role: "user", Content: "test"}}); !errors.Is(err, ai.ErrNetwork) {
		t.Fatal("network error not classified")
	}
	if _, err := c.Complete(context.Background(), nil); !errors.Is(err, ai.ErrInput) {
		t.Fatal("empty input accepted")
	}
	if _, err := ai.NewClient(config.AI{}); !errors.Is(err, config.ErrAIConfig) {
		t.Fatal("invalid config accepted")
	}
}

func TestUntrustedTLS(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	c := client(t, server.URL, rand.Text(), time.Second)
	_, err := c.Complete(context.Background(), []ai.Message{{Role: "user", Content: "test"}})
	if !errors.Is(err, ai.ErrNetwork) || requests.Load() != 0 {
		t.Fatal("untrusted server received a request or TLS error was not classified")
	}
}

func TestFakeAndSuggestionShape(t *testing.T) {
	ctx := context.Background()
	for _, raw := range []string{"", "not JSON", "[]", `{"category":"general","summary":"s"}`, `{"category":"general","summary":5,"reply_suggestion":"x"}`} {
		text, err := (aitest.Fake{Content: raw}).Complete(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ai.ParseSuggestion(text); !errors.Is(err, ai.ErrOutput) {
			t.Fatal("invalid suggestion accepted")
		}
	}
	if _, err := (aitest.Fake{Err: ai.ErrNetwork}).Complete(ctx, nil); !errors.Is(err, ai.ErrNetwork) {
		t.Fatal("fake lost planned failure")
	}
}
