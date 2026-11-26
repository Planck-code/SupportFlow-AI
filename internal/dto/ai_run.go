package dto

import (
	"encoding/json"
	"time"
)

type AIRun struct {
	ID              uint64          `json:"id"`
	TicketID        uint64          `json:"ticket_id"`
	RequestedBy     uint64          `json:"requested_by"`
	Status          string          `json:"status"`
	Provider        string          `json:"provider"`
	Model           string          `json:"model"`
	InputSnapshotID string          `json:"input_snapshot_id"`
	InputTruncated  bool            `json:"input_truncated"`
	IsStale         bool            `json:"is_stale" gorm:"-"`
	Result          json.RawMessage `json:"result"`
	ErrorCategory   *string         `json:"error_category"`
	CreatedAt       time.Time       `json:"created_at"`
	FinishedAt      *time.Time      `json:"finished_at"`
}
