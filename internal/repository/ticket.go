package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/Planck-code/SupportFlow-AI/internal/dto"
	"github.com/Planck-code/SupportFlow-AI/internal/model"
)

type Tickets struct{ db *gorm.DB }

func NewTickets(db *gorm.DB) *Tickets { return &Tickets{db} }

func (r *Tickets) Transaction(ctx context.Context, work func(*Tickets) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return work(NewTickets(tx)) })
}

func (r *Tickets) Insert(ctx context.Context, t *model.Ticket) error {
	return r.db.WithContext(ctx).Create(t).Error
}

func (r *Tickets) InsertEvent(ctx context.Context, e *model.TicketEvent) error {
	return r.db.WithContext(ctx).Create(e).Error
}

// Scope is determined by the service from the authenticated identity.
type TicketScope struct{ RequesterID, AssigneeID uint64 }

func (r *Tickets) visible(ctx context.Context, s TicketScope) *gorm.DB {
	q := r.db.WithContext(ctx).Model(&model.Ticket{})
	if s.RequesterID != 0 {
		q = q.Where("requester_id = ?", s.RequesterID)
	}
	if s.AssigneeID != 0 {
		q = q.Where("assignee_id = ?", s.AssigneeID)
	}
	return q
}

func (r *Tickets) Find(ctx context.Context, s TicketScope, id uint64) (model.Ticket, error) {
	var t model.Ticket
	err := r.visible(ctx, s).Where("id = ?", id).Take(&t).Error
	return t, err
}

func pageRows[T any](q *gorm.DB, columns, order string, p dto.Page) ([]T, int64, error) {
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]T, 0)
	err := q.Select(columns).Order(order).Offset((p.Number - 1) * p.Size).Limit(p.Size).Find(&items).Error
	return items, total, err
}

func (r *Tickets) List(ctx context.Context, s TicketScope, f dto.TicketFilter, p dto.Page) ([]dto.TicketSummary, int64, error) {
	q := r.visible(ctx, s)
	if f.Category != "" {
		q = q.Where("category = ?", f.Category)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.AssigneeID != 0 {
		q = q.Where("assignee_id = ?", f.AssigneeID)
	}
	if f.Unassigned {
		q = q.Where("assignee_id IS NULL")
	}
	return pageRows[dto.TicketSummary](q, "id, requester_id, assignee_id, title, category, priority, status, created_at, updated_at", "created_at DESC, id DESC", p)
}

func (r *Tickets) Queue(ctx context.Context, category string, p dto.Page) ([]dto.QueueItem, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.Ticket{}).Where("status = 'open' AND assignee_id IS NULL")
	if category != "" {
		q = q.Where("category = ?", category)
	}
	return pageRows[dto.QueueItem](q, "id, title, category, priority, created_at", "created_at DESC, id DESC", p)
}

func (r *Tickets) Events(ctx context.Context, s TicketScope, id uint64, p dto.Page) ([]dto.Event, int64, error) {
	parent := r.visible(ctx, s).Where("id = ?", id)
	var count int64
	if err := parent.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	if count == 0 {
		return nil, 0, gorm.ErrRecordNotFound
	}
	q := r.db.WithContext(ctx).Model(&model.TicketEvent{}).Where("ticket_id IN (?)", parent.Select("id"))
	return pageRows[dto.Event](q, "id, ticket_id, actor_id, event_type, metadata, created_at", "id ASC", p)
}
