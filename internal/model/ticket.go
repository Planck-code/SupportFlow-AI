package model

import (
	"encoding/json"
	"time"
)

type Ticket struct {
	ID          uint64
	RequesterID uint64
	AssigneeID  *uint64
	Title       string
	Description string
	Category    string
	Priority    string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type TicketEvent struct {
	ID        uint64
	TicketID  uint64
	ActorID   uint64
	EventType string
	Metadata  json.RawMessage
	CreatedAt time.Time
}

type TicketMessage struct {
	ID        uint64
	TicketID  uint64
	AuthorID  uint64
	Content   string
	CreatedAt time.Time
}
