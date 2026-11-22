package handler

import (
	"strings"
	"testing"
)

// The endpoint has a closed contract even though public registration uses Gin's normal binding.
func TestDecodeCreateStaff(t *testing.T) {
	fields := `"email":"agent@example.test","password":" pass word ","display_name":"Agent","role":"agent"`
	valid := "{" + fields + "}"
	got, err := decodeStaff(strings.NewReader(valid))
	if err != nil || got.Password != " pass word " || got.Role != "agent" {
		t.Fatal("valid request changed or rejected")
	}
	for _, body := range []string{
		"{}", "null", "[]", valid + " {}", valid + " trailing", `{"email":1}`, `{"role":null}`,
		"{" + fields + `,"id":7}`, "{" + fields + `,"password_hash":"fake"}`, "{" + fields + `,"requester_id":1}`, "{" + fields + `,"actor_id":1}`, "{" + fields + `,"role":"admin"}`, strings.Replace(valid, `"role"`, `"ROLE"`, 1),
	} {
		if _, err := decodeStaff(strings.NewReader(body)); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
}
