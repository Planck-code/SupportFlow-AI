package service

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"gorm.io/gorm"

	"github.com/Planck-code/SupportFlow-AI/internal/dto"
	"github.com/Planck-code/SupportFlow-AI/internal/model"
	"github.com/Planck-code/SupportFlow-AI/internal/repository"
)

type TicketAction string

const (
	ChangePriority TicketAction = "priority"
	Resolve        TicketAction = "resolve"
	Reopen         TicketAction = "reopen"
	Close          TicketAction = "close"
)

var ErrLifecycleConflict = errors.New("action unavailable in current ticket state")

type transition struct {
	roles           []string
	from, to, event string
}

var transitions = map[TicketAction]transition{
	ChangePriority: {[]string{model.RoleAgent, model.RoleAdmin}, "in_progress", "in_progress", "priority_changed"},
	Resolve:        {[]string{model.RoleAgent}, "in_progress", "resolved", "resolved"},
	Reopen:         {[]string{model.RoleUser}, "resolved", "in_progress", "reopened"},
	Close:          {[]string{model.RoleUser, model.RoleAdmin}, "resolved", "closed", "closed"},
}

func (a TicketAction) Roles() []string { return slices.Clone(transitions[a].roles) }

func (r transition) authorize(ticket model.Ticket, actor uint64, role string) error {
	if !slices.Contains(r.roles, role) {
		return ErrTicketForbidden
	}
	switch role {
	case model.RoleAgent:
		if ticket.AssigneeID == nil || *ticket.AssigneeID != actor {
			return gorm.ErrRecordNotFound
		}
	case model.RoleUser:
		if ticket.RequesterID != actor {
			return gorm.ErrRecordNotFound
		}
	}
	if ticket.Status != r.from {
		return ErrLifecycleConflict
	}
	return nil
}

func (s *TicketService) ApplyAction(ctx context.Context, actor, id uint64, action TicketAction, priority string) (dto.TicketDetail, error) {
	rule, ok := transitions[action]
	if !ok || (action == ChangePriority && priority != "normal" && priority != "high") {
		return dto.TicketDetail{}, ErrInvalidInput
	}
	var result dto.TicketDetail
	err := s.repo.Transaction(ctx, func(tx *repository.Tickets) error {
		ticket, err := tx.Lock(ctx, id)
		if err != nil {
			return err
		}
		role, err := tx.LockUserRole(ctx, actor)
		if err != nil {
			return err
		}
		if err := rule.authorize(ticket, actor, role); err != nil {
			return err
		}

		var changes map[string]string
		if action == ChangePriority {
			if ticket.Priority == priority {
				result = dto.Detail(ticket)
				return nil
			}
			changes = map[string]string{"from_priority": ticket.Priority, "to_priority": priority}
			err = tx.SetPriority(ctx, id, priority)
		} else {
			changes = map[string]string{"from_status": ticket.Status, "to_status": rule.to}
			err = tx.SetStatus(ctx, id, rule.to)
		}
		if err != nil {
			return err
		}
		metadata, err := json.Marshal(changes)
		if err != nil {
			return err
		}
		if err := tx.InsertEvent(ctx, &model.TicketEvent{
			TicketID: id, ActorID: actor, EventType: rule.event, Metadata: metadata,
		}); err != nil {
			return err
		}
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
