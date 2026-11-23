package service

import (
	"context"
	"encoding/json"
	"errors"

	"gorm.io/gorm"

	"github.com/Planck-code/SupportFlow-AI/internal/dto"
	"github.com/Planck-code/SupportFlow-AI/internal/model"
	"github.com/Planck-code/SupportFlow-AI/internal/repository"
)

var ErrReplyConflict = errors.New("ticket is not open for replies")

func canReply(ticket model.Ticket, actor uint64, role string) error {
	switch role {
	case model.RoleUser:
		if ticket.RequesterID != actor {
			return gorm.ErrRecordNotFound
		}
	case model.RoleAgent:
		if ticket.AssigneeID == nil || *ticket.AssigneeID != actor {
			return gorm.ErrRecordNotFound
		}
	default:
		return ErrTicketForbidden
	}
	if ticket.Status != "open" && ticket.Status != "in_progress" {
		return ErrReplyConflict
	}
	return nil
}

func (s *TicketService) Reply(ctx context.Context, user dto.UserResponse, id uint64, content string) (dto.Message, error) {
	if !validText(content, 5000) {
		return dto.Message{}, ErrInvalidInput
	}
	msg := model.TicketMessage{TicketID: id, AuthorID: user.ID, Content: content}
	err := s.repo.Transaction(ctx, func(tx *repository.Tickets) error {
		ticket, err := tx.Lock(ctx, id)
		if err != nil {
			return err
		}
		role, err := tx.LockUserRole(ctx, user.ID)
		if err != nil {
			return err
		}
		if err := canReply(ticket, user.ID, role); err != nil {
			return err
		}
		if err := tx.InsertMessage(ctx, &msg); err != nil {
			return err
		}
		metadata, err := json.Marshal(map[string]uint64{"message_id": msg.ID})
		if err != nil {
			return err
		}
		return tx.InsertEvent(ctx, &model.TicketEvent{
			TicketID: id, ActorID: user.ID, EventType: "message_added", Metadata: metadata,
		})
	})
	if err != nil {
		return dto.Message{}, err
	}
	return dto.Message{ID: msg.ID, TicketID: id, AuthorID: user.ID, Content: content, CreatedAt: msg.CreatedAt}, nil
}

func (s *TicketService) Messages(ctx context.Context, user dto.UserResponse, id uint64, p dto.Page) ([]dto.Message, int64, error) {
	scope, err := ticketScope(user)
	if err != nil {
		return nil, 0, err
	}
	var items []dto.Message
	var total int64
	// Check visibility, count and fetch the page from the same read view.
	err = s.repo.Transaction(ctx, func(tx *repository.Tickets) error {
		var err error
		items, total, err = tx.Messages(ctx, scope, id, p)
		return err
	})
	return items, total, err
}
