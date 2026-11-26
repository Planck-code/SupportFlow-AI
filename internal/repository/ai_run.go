package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Planck-code/SupportFlow-AI/internal/dto"
	"github.com/Planck-code/SupportFlow-AI/internal/model"
	"gorm.io/gorm"
)

// Caller holds the ticket lock. Message writers acquire it before inserting.
func (r *Tickets) RecentMessages(ctx context.Context, ticketID uint64, limit int) ([]model.TicketMessage, error) {
	var rows []model.TicketMessage
	err := r.db.WithContext(ctx).Select("id", "author_id", "content").Where("ticket_id = ?", ticketID).Order("id DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

func (r *Tickets) InsertRun(ctx context.Context, run *model.AIRun) error {
	return r.db.WithContext(ctx).Create(run).Error
}

func (r *Tickets) FinishRun(ctx context.Context, id uint64, result json.RawMessage, category string) error {
	fields := map[string]any{"finished_at": gorm.Expr("UTC_TIMESTAMP(6)")}
	if category == "" {
		fields["status"], fields["result"] = "succeeded", string(result)
	} else {
		fields["status"], fields["error_category"] = "failed", category
	}
	update := r.db.WithContext(ctx).Model(&model.AIRun{}).Where("id = ? AND status = 'running'", id).Updates(fields)
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected != 1 {
		return errors.New("AI run was not finalized")
	}
	return nil
}

// Read the current role, assignment and run in one SQL statement.
func (r *Tickets) VisibleRun(ctx context.Context, actor, ticketID, runID uint64) (dto.AIRun, error) {
	var run dto.AIRun
	err := r.db.WithContext(ctx).Table("ai_runs").Select("ai_runs.*").
		Joins("JOIN tickets ON tickets.id = ai_runs.ticket_id").
		Joins("JOIN users AS viewer ON viewer.id = ?", actor).
		Where("ai_runs.ticket_id = ? AND ai_runs.id = ?", ticketID, runID).
		Where("(viewer.role = 'admin' OR (viewer.role = 'agent' AND tickets.assignee_id = viewer.id))").Take(&run).Error
	return run, err
}
