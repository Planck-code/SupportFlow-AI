package ai

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSuggestionContract(t *testing.T) {
	for _, c := range []string{"general", "account", "billing", "technical"} {
		in := Suggestion{c, strings.Repeat("界", 1000), strings.Repeat("🙂", 5000)}
		raw, _ := json.Marshal(in)
		if got, err := ParseSuggestion(string(raw)); err != nil || got != in {
			t.Fatal("valid boundary rejected")
		}
	}
	base := `{"category":"billing","summary":"s","reply_suggestion":"r"}`
	for _, bad := range []string{
		base + "{}", strings.Replace(base, "billing", "urgent", 1), strings.Replace(base, `"summary":"s"`, `"summary":""`, 1),
		strings.Replace(base, `"s"`, `"s","summary":"x"`, 1), strings.Replace(base, "summary", "Summary", 1),
		strings.Replace(base, `"r"`, `"r","extra":1`, 1), strings.Replace(base, `"s"`, "null", 1),
		strings.Replace(base, `"s"`, "7", 1), strings.Replace(base, `"s"`, `" "`, 1),
		strings.Replace(base, `"r"`, `"`+strings.Repeat("界", 5001)+`"`, 1),
		strings.Replace(base, `"s"`, `"`+strings.Repeat("🙂", 1001)+`"`, 1),
	} {
		if _, err := ParseSuggestion(bad); !errors.Is(err, ErrOutput) {
			t.Fatal("invalid suggestion accepted")
		}
	}
}
