package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/Planck-code/SupportFlow-AI/internal/dto"
	"github.com/Planck-code/SupportFlow-AI/internal/model"
	"github.com/Planck-code/SupportFlow-AI/internal/repository"
)

var ErrTicketForbidden = errors.New("ticket operation forbidden")
var ErrAssignmentConflict = errors.New("ticket action unavailable")
var ErrInvalidAssignee = errors.New("assignee must be an existing agent")

func (s *TicketService) Claim(ctx context.Context, u dto.UserResponse, id uint64) (dto.TicketDetail, error) {
	return s.changeAssignment(ctx, u.ID, id, u.ID, model.RoleAgent)
}

func (s *TicketService) Assign(ctx context.Context, u dto.UserResponse, id, target uint64) (dto.TicketDetail, error) {
	return s.changeAssignment(ctx, u.ID, id, target, model.RoleAdmin)
}

func (s *TicketService) changeAssignment(ctx context.Context, actor, id, target uint64, requiredRole string) (dto.TicketDetail, error) {
	var result dto.TicketDetail
	err := s.repo.Transaction(ctx, func(tx *repository.Tickets) error {
		claiming := requiredRole == model.RoleAgent
		var snapshotRole string
		var err error
		if claiming {
			snapshotRole, err = tx.ClaimantSnapshot(ctx, actor)
			if err != nil {
				return err
			}
		}
		ticket, err := tx.Lock(ctx, id)
		if err != nil {
			return err
		}
		role, err := tx.LockUserRole(ctx, actor)
		if err != nil {
			return err
		}
		if role != requiredRole {
			return ErrTicketForbidden
		}

		if claiming && ticket.AssigneeID == nil && ticket.Status != "open" {
			return gorm.ErrRecordNotFound
		}
		if claiming && ticket.AssigneeID != nil && *ticket.AssigneeID != actor {
			wasQueued, err := tx.WasInQueue(ctx, id)
			if err != nil {
				return err
			}
			if wasQueued && snapshotRole == model.RoleAgent {
				return ErrAssignmentConflict
			}
			return gorm.ErrRecordNotFound
		}
		event, err := assignmentEvent(ticket, claiming, target)
		if err != nil {
			return err
		}
		if !claiming {
			role, err := tx.LockUserRole(ctx, target)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalidAssignee
			}
			if err != nil {
				return err
			}
			if role != model.RoleAgent {
				return ErrInvalidAssignee
			}
		}
		if event == "" {
			result = dto.Detail(ticket)
			return nil
		}

		metadata, err := json.Marshal(map[string]any{
			"from_status": ticket.Status, "to_status": "in_progress",
			"from_assignee_id": ticket.AssigneeID, "to_assignee_id": target,
		})
		if err != nil {
			return err
		}
		if err := tx.SetAssignment(ctx, id, target); err != nil {
			return err
		}
		if err := tx.InsertEvent(ctx, &model.TicketEvent{TicketID: id, ActorID: actor, EventType: event, Metadata: metadata}); err != nil {
			return err
		}
		// Read back the persisted timestamp, including MySQL's microsecond precision.
		updated, err := tx.Find(ctx, repository.TicketScope{}, id)
		if err != nil {
			return err
		}
		result = dto.Detail(updated)
		return nil
	})
	if err != nil {
		return dto.TicketDetail{}, err
	}
	return result, nil
}

// Empty event means a valid, idempotent assignment.
func assignmentEvent(t model.Ticket, claiming bool, target uint64) (string, error) {
	if t.Status == "open" && t.AssigneeID == nil {
		if claiming {
			return "claimed", nil
		}
		return "assigned", nil
	}
	if !claiming && t.Status == "in_progress" && t.AssigneeID != nil {
		if *t.AssigneeID == target {
			return "", nil
		}
		return "reassigned", nil
	}
	return "", ErrAssignmentConflict
}

type TicketService struct{ repo *repository.Tickets }

func NewTicketService(repo *repository.Tickets) *TicketService { return &TicketService{repo} }

func validCategory(s string) bool {
	switch s {
	case "general", "account", "technical", "billing":
		return true
	}
	return false
}

func validText(s string, max int) bool {
	return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= max
}

func (s *TicketService) Create(ctx context.Context, u dto.UserResponse, in dto.CreateTicket) (dto.TicketDetail, error) {
	if u.Role != model.RoleUser || u.ID == 0 {
		return dto.TicketDetail{}, ErrTicketForbidden
	}
	if !validText(in.Title, 120) || !validText(in.Description, 10000) || in.Category == nil || !validCategory(*in.Category) {
		return dto.TicketDetail{}, ErrInvalidInput
	}
	t := model.Ticket{RequesterID: u.ID, Title: in.Title, Description: in.Description, Category: *in.Category, Priority: "normal", Status: "open"}
	err := s.repo.Transaction(ctx, func(tx *repository.Tickets) error {
		if err := tx.Insert(ctx, &t); err != nil {
			return err
		}
		return tx.InsertEvent(ctx, &model.TicketEvent{TicketID: t.ID, ActorID: u.ID, EventType: "created", Metadata: []byte(`{"status":"open"}`)})
	})
	if err != nil {
		return dto.TicketDetail{}, err
	}
	return dto.Detail(t), nil
}

func ticketScope(u dto.UserResponse) (repository.TicketScope, error) {
	if u.ID != 0 {
		switch u.Role {
		case model.RoleUser:
			return repository.TicketScope{RequesterID: u.ID}, nil
		case model.RoleAgent:
			return repository.TicketScope{AssigneeID: u.ID}, nil
		case model.RoleAdmin:
			return repository.TicketScope{}, nil
		}
	}
	return repository.TicketScope{}, ErrTicketForbidden
}

func (s *TicketService) List(ctx context.Context, u dto.UserResponse, f dto.TicketFilter, p dto.Page) ([]dto.TicketSummary, int64, error) {
	scope, err := ticketScope(u)
	if err != nil {
		return nil, 0, err
	}
	if f.AssigneeID != 0 || f.Unassigned {
		if u.Role != model.RoleAdmin || f.AssigneeID != 0 && f.Unassigned {
			return nil, 0, ErrInvalidInput
		}
	}
	switch f.Status {
	case "", "open", "in_progress", "resolved", "closed":
	default:
		return nil, 0, ErrInvalidInput
	}
	if f.Category != "" && !validCategory(f.Category) {
		return nil, 0, ErrInvalidInput
	}
	return s.repo.List(ctx, scope, f, p)
}

func (s *TicketService) Queue(ctx context.Context, u dto.UserResponse, category string, p dto.Page) ([]dto.QueueItem, int64, error) {
	if u.Role != model.RoleAgent || u.ID == 0 {
		return nil, 0, ErrTicketForbidden
	}
	if category != "" && !validCategory(category) {
		return nil, 0, ErrInvalidInput
	}
	return s.repo.Queue(ctx, category, p)
}

func (s *TicketService) Detail(ctx context.Context, u dto.UserResponse, id uint64) (dto.TicketDetail, error) {
	scope, err := ticketScope(u)
	if err != nil {
		return dto.TicketDetail{}, err
	}
	t, err := s.repo.Find(ctx, scope, id)
	return dto.Detail(t), err
}

func (s *TicketService) Events(ctx context.Context, u dto.UserResponse, id uint64, p dto.Page) ([]dto.Event, int64, error) {
	scope, err := ticketScope(u)
	if err != nil {
		return nil, 0, err
	}
	return s.repo.Events(ctx, scope, id, p)
}
