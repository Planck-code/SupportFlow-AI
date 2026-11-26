package ai

import (
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"
)

// Suggestion is the proposed JSON payload inside the assistant's text.
// Only validated suggestions may be persisted.
type Suggestion struct {
	Category        string `json:"category"`
	Summary         string `json:"summary"`
	ReplySuggestion string `json:"reply_suggestion"`
}

func ParseSuggestion(content string) (Suggestion, error) {
	var s Suggestion
	if !utf8.ValidString(content) {
		return Suggestion{}, ErrOutput
	}
	d := json.NewDecoder(strings.NewReader(content))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return Suggestion{}, ErrOutput
	}
	fields := map[string]*string{"category": &s.Category, "summary": &s.Summary, "reply_suggestion": &s.ReplySuggestion}
	for d.More() {
		key, err := d.Token()
		name, ok := key.(string)
		dest := fields[name]
		if err != nil || !ok || dest == nil || d.Decode(dest) != nil {
			return Suggestion{}, ErrOutput
		}
		delete(fields, name)
	}
	if end, err := d.Token(); err != nil || end != json.Delim('}') || len(fields) != 0 {
		return Suggestion{}, ErrOutput
	}
	if _, err := d.Token(); err != io.EOF {
		return Suggestion{}, ErrOutput
	}
	switch s.Category {
	case "general", "account", "billing", "technical":
	default:
		return Suggestion{}, ErrOutput
	}
	if strings.TrimSpace(s.Summary) == "" || strings.TrimSpace(s.ReplySuggestion) == "" ||
		utf8.RuneCountInString(s.Summary) > 1000 || utf8.RuneCountInString(s.ReplySuggestion) > 5000 {
		return Suggestion{}, ErrOutput
	}
	return s, nil
}
