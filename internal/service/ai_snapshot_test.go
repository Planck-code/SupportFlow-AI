package service

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Planck-code/SupportFlow-AI/internal/ai"
	"github.com/Planck-code/SupportFlow-AI/internal/ai/aitest"
	"github.com/Planck-code/SupportFlow-AI/internal/model"
)

func TestSnapshot(t *testing.T) {
	ticket := model.Ticket{ID: 7, RequesterID: 1, Title: "a", Description: "b"}
	rows := make([]model.TicketMessage, 21)
	for i := range rows {
		rows[i] = model.TicketMessage{ID: uint64(21 - i), AuthorID: 1, Content: strings.Repeat("字", i+1)}
	}
	messages, id, truncated := buildSnapshot(ticket, rows)
	again, id2, _ := buildSnapshot(ticket, rows)
	if !truncated || id != id2 || !reflect.DeepEqual(messages, again) || !strings.HasPrefix(id, "v1:") {
		t.Fatal("unstable snapshot")
	}
	var data snapshotData
	if err := json.Unmarshal([]byte(messages[1].Content), &data); err != nil || len(data.Messages) != 20 || data.Messages[0].Content != rows[19].Content || data.Messages[19].Content != rows[0].Content {
		t.Fatal("recent context out of order")
	}
	rows[0].ID++
	_, id2, _ = buildSnapshot(ticket, rows)
	if id2 == id {
		t.Fatal("latest message id missing from digest")
	}
	for _, content := range []string{strings.Repeat("🙂", 5000), strings.Repeat("<\n", 2500)} {
		rows[0].Content = content
		ticket.Description = strings.Repeat(content, 2)
		m, _, cut := buildSnapshot(ticket, rows)
		if !cut || !json.Valid([]byte(m[1].Content)) || utf8.RuneCountInString(m[0].Content+m[1].Content) > maxInputChars {
			t.Fatal("invalid truncated input")
		}
	}
	m, _, cut := buildSnapshot(model.Ticket{Title: "t"}, nil)
	if cut || len(m) != 2 || m[0].Role != "system" || m[1].Role != "user" {
		t.Fatal("unexpected empty conversation")
	}
}

func TestAnalyzeWithFake(t *testing.T) {
	for _, tc := range []struct {
		fake aitest.Fake
		want ModelFailure
	}{
		{aitest.Fake{Content: `{"category":"account","summary":"s","reply_suggestion":"r"}`}, ""},
		{aitest.Fake{Err: ai.ErrNetwork}, "network"},
		{aitest.Fake{Err: context.Canceled}, "canceled"},
		{aitest.Fake{Content: "bad"}, "invalid_output"},
	} {
		s := NewAIRuns(nil, tc.fake, "mock")
		result, got := s.analyze(context.Background(), nil)
		if got != tc.want || (len(result) > 0) != (got == "") {
			t.Fatalf("unexpected outcome %s", got)
		}
	}
}
