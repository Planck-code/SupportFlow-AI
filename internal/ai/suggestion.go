package ai

import (
	"encoding/json"
	"strings"
)

// Suggestion is the proposed JSON payload inside the assistant's text.
// Enum and business length checks belong to the future ticket service.
type Suggestion struct {
	Category        string `json:"category"`
	Summary         string `json:"summary"`
	ReplySuggestion string `json:"reply_suggestion"`
}

func ParseSuggestion(content string) (Suggestion, error) {
	var s Suggestion
	if json.Unmarshal([]byte(content), &s) != nil || strings.TrimSpace(s.Category) == "" ||
		strings.TrimSpace(s.Summary) == "" || strings.TrimSpace(s.ReplySuggestion) == "" {
		return Suggestion{}, ErrOutput
	}
	return s, nil
}
