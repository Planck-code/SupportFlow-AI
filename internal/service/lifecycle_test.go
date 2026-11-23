package service

import (
	"errors"
	"testing"

	"gorm.io/gorm"

	"github.com/Planck-code/SupportFlow-AI/internal/model"
)

func TestTransitionAuthorization(t *testing.T) {
	owner, agent := uint64(11), uint64(12)
	for _, tc := range []struct {
		name        string
		action      TicketAction
		role, state string
		actor       uint64
		want        error
	}{
		{"agent changes priority", ChangePriority, "agent", "in_progress", agent, nil},
		{"admin changes priority", ChangePriority, "admin", "in_progress", 20, nil},
		{"same priority after closure still conflicts", ChangePriority, "admin", "closed", 20, ErrLifecycleConflict},
		{"unassigned ticket hidden", Resolve, "agent", "open", agent, gorm.ErrRecordNotFound},
		{"agent resolves", Resolve, "agent", "in_progress", agent, nil},
		{"former assignee", Resolve, "agent", "resolved", 99, gorm.ErrRecordNotFound},
		{"admin cannot resolve", Resolve, "admin", "in_progress", 20, ErrTicketForbidden},
		{"repeat resolve", Resolve, "agent", "resolved", agent, ErrLifecycleConflict},
		{"owner reopens", Reopen, "user", "resolved", owner, nil},
		{"other user before state check", Reopen, "user", "closed", 99, gorm.ErrRecordNotFound},
		{"repeat reopen", Reopen, "user", "in_progress", owner, ErrLifecycleConflict},
		{"agent cannot reopen", Reopen, "agent", "resolved", agent, ErrTicketForbidden},
		{"closed is terminal", Reopen, "user", "closed", owner, ErrLifecycleConflict},
		{"owner closes", Close, "user", "resolved", owner, nil},
		{"admin closes", Close, "admin", "resolved", 20, nil},
		{"agent cannot close", Close, "agent", "resolved", agent, ErrTicketForbidden},
		{"cannot skip resolve", Close, "user", "open", owner, ErrLifecycleConflict},
		{"other user cannot close", Close, "user", "resolved", 99, gorm.ErrRecordNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ticket := model.Ticket{RequesterID: owner, Status: tc.state}
			if tc.state != "open" {
				ticket.AssigneeID = &agent
			}
			if got := transitions[tc.action].authorize(ticket, tc.actor, tc.role); !errors.Is(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
