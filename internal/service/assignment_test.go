package service

import (
	"errors"
	"testing"

	"github.com/Planck-code/SupportFlow-AI/internal/model"
)

func TestAssignmentTransitions(t *testing.T) {
	agent := uint64(17)
	for _, tc := range []struct {
		name, status string
		owner        *uint64
		claim        bool
		target       uint64
		event        string
		conflict     bool
	}{
		{"claim", "open", nil, true, 17, "claimed", false},
		{"first allocation", "open", nil, false, 17, "assigned", false},
		{"transfer", "in_progress", &agent, false, 18, "reassigned", false},
		{"no change", "in_progress", &agent, false, 17, "", false},
		{"duplicate claim", "in_progress", &agent, true, 17, "", true},
		{"resolved", "resolved", &agent, false, 18, "", true},
		{"closed same owner", "closed", &agent, false, 17, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event, err := assignmentEvent(model.Ticket{Status: tc.status, AssigneeID: tc.owner}, tc.claim, tc.target)
			if event != tc.event || errors.Is(err, ErrAssignmentConflict) != tc.conflict || (!tc.conflict && err != nil) {
				t.Fatal("unexpected transition", event, err)
			}
		})
	}
}
