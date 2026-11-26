package model

import (
	"encoding/json"
	"time"
)

type AIRun struct {
	ID              uint64
	TicketID        uint64
	RequestedBy     uint64
	Status          string
	Provider        string
	Model           string
	InputSnapshotID string
	InputTruncated  bool
	Result          json.RawMessage
	ErrorCategory   *string
	CreatedAt       time.Time
	FinishedAt      *time.Time
}

func (AIRun) TableName() string { return "ai_runs" }
