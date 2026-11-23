package service

import (
	"errors"
	"testing"

	"gorm.io/gorm"

	"github.com/Planck-code/SupportFlow-AI/internal/model"
)

func TestReplyPermission(t *testing.T) {
	agent := uint64(2)
	for _, tc := range []struct {
		role, status string
		id           uint64
		want         error
	}{
		{"user", "open", 1, nil}, {"agent", "open", 2, gorm.ErrRecordNotFound},
		{"agent", "in_progress", 2, nil}, {"agent", "in_progress", 3, gorm.ErrRecordNotFound},
		{"user", "resolved", 1, ErrReplyConflict}, {"agent", "closed", 2, ErrReplyConflict},
		{"user", "closed", 3, gorm.ErrRecordNotFound}, {"admin", "in_progress", 4, ErrTicketForbidden},
	} {
		ticket := model.Ticket{RequesterID: 1, Status: tc.status}
		if tc.status != "open" {
			ticket.AssigneeID = &agent
		}
		if err := canReply(ticket, tc.id, tc.role); !errors.Is(err, tc.want) {
			t.Errorf("role=%s status=%s id=%d: got %v, want %v", tc.role, tc.status, tc.id, err, tc.want)
		}
	}
}
