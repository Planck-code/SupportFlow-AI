package dto

import (
	"encoding/json"
	"time"

	"github.com/Planck-code/SupportFlow-AI/internal/model"
)

type CreateTicket struct {
	Title       string  `json:"title" binding:"required"`
	Description string  `json:"description" binding:"required"`
	Category    *string `json:"category"`
}

type QueueItem struct {
	ID        uint64    `json:"id"`
	Title     string    `json:"title"`
	Category  string    `json:"category"`
	Priority  string    `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
}

type TicketSummary struct {
	QueueItem
	RequesterID uint64    `json:"requester_id"`
	AssigneeID  *uint64   `json:"assignee_id"`
	Status      string    `json:"status"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type TicketDetail struct {
	TicketSummary
	Description string `json:"description"`
}

func Detail(t model.Ticket) TicketDetail {
	return TicketDetail{TicketSummary: TicketSummary{QueueItem: QueueItem{ID: t.ID, Title: t.Title, Category: t.Category, Priority: t.Priority, CreatedAt: t.CreatedAt}, RequesterID: t.RequesterID, AssigneeID: t.AssigneeID, Status: t.Status, UpdatedAt: t.UpdatedAt}, Description: t.Description}
}

type Event struct {
	ID        uint64          `json:"id"`
	TicketID  uint64          `json:"ticket_id"`
	ActorID   uint64          `json:"actor_id"`
	EventType string          `json:"event_type"`
	Metadata  json.RawMessage `json:"metadata"`
	CreatedAt time.Time       `json:"created_at"`
}

type Page struct{ Number, Size int }

type TicketFilter struct {
	Status, Category string
	AssigneeID       uint64
	Unassigned       bool
}
