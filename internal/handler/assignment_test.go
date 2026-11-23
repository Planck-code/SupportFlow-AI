package handler

import (
	"strings"
	"testing"
)

func TestAssignmentBody(t *testing.T) {
	for _, tc := range []struct {
		body  string
		claim bool
		want  uint64
		ok    bool
	}{
		{"", true, 0, true}, {"{}", true, 0, true}, {`{"actor_id":1}`, true, 0, false}, {"null", true, 0, false},
		{`{"assignee_id":18446744073709551615}`, false, ^uint64(0), true},
		{"", false, 0, false}, {"{}", false, 0, false}, {`{"assignee_id":null}`, false, 0, false}, {`{"assignee_id":0}`, false, 0, false}, {`{"assignee_id":-1}`, false, 0, false}, {`{"assignee_id":"1"}`, false, 0, false}, {`{"assignee_id":1.2}`, false, 0, false}, {`{"assignee_id":1,"assignee_id":2}`, false, 0, false}, {`{"assignee_id":2,"status":"closed"}`, false, 0, false}, {`{"assignee_id":1} {}`, false, 0, false},
	} {
		id, ok := assignmentBody(strings.NewReader(tc.body), tc.claim)
		if id != tc.want || ok != tc.ok {
			t.Errorf("body %q got %d/%v", tc.body, id, ok)
		}
	}
}
