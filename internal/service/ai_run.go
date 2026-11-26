package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Planck-code/SupportFlow-AI/internal/ai"
	"github.com/Planck-code/SupportFlow-AI/internal/config"
	"github.com/Planck-code/SupportFlow-AI/internal/dto"
	"github.com/Planck-code/SupportFlow-AI/internal/model"
	"github.com/Planck-code/SupportFlow-AI/internal/repository"
	"gorm.io/gorm"
)

var ErrAIUnavailable = errors.New("AI unavailable")

type ModelFailure string

func (e ModelFailure) Error() string { return string(e) }

type AIRuns struct {
	repo      *repository.Tickets
	client    ai.Client
	modelName string
}

func NewAIRuns(repo *repository.Tickets, client ai.Client, modelName string) *AIRuns {
	return &AIRuns{repo: repo, client: client, modelName: modelName}
}

func (s *AIRuns) Get(ctx context.Context, actor, ticketID, runID uint64) (dto.AIRun, error) {
	// VisibleRun's ordinary SELECT establishes the repeatable-read view for
	// authorization, ticket content and messages, including POST's final read.
	var run dto.AIRun
	err := s.repo.Transaction(ctx, func(tx *repository.Tickets) error {
		visible, err := tx.VisibleRun(ctx, actor, ticketID, runID)
		if err != nil {
			return err
		}
		ticket, err := tx.Find(ctx, repository.TicketScope{}, ticketID)
		if err != nil {
			return err
		}
		rows, err := tx.RecentMessages(ctx, ticketID, recentLimit+1)
		if err != nil {
			return err
		}
		_, id, _ := buildSnapshot(ticket, rows)
		visible.IsStale = visible.InputSnapshotID != id
		run = visible
		return nil
	})
	if err != nil {
		return dto.AIRun{}, err
	}
	return run, nil
}

func (s *AIRuns) Start(ctx context.Context, actor, ticketID uint64) (dto.AIRun, error) {
	run := model.AIRun{TicketID: ticketID, RequestedBy: actor, Status: "running", Provider: "openai_compatible", Model: s.modelName}
	var messages []ai.Message
	err := s.repo.Transaction(ctx, func(tx *repository.Tickets) error {
		ticket, err := tx.Lock(ctx, ticketID)
		if err != nil {
			return err
		}
		role, err := tx.LockUserRole(ctx, actor)
		if err != nil {
			return err
		}
		if role != model.RoleAgent {
			return ErrTicketForbidden
		}
		if ticket.AssigneeID == nil || *ticket.AssigneeID != actor {
			return gorm.ErrRecordNotFound
		}
		if ticket.Status != "in_progress" {
			return ErrLifecycleConflict
		}
		if s.client == nil {
			return ErrAIUnavailable
		}
		rows, err := tx.RecentMessages(ctx, ticketID, recentLimit+1)
		if err != nil {
			return err
		}
		messages, run.InputSnapshotID, run.InputTruncated = buildSnapshot(ticket, rows)
		return tx.InsertRun(ctx, &run)
	})
	if err != nil {
		return dto.AIRun{}, err
	}

	result, failure := s.analyze(ctx, messages)
	// A disconnected caller cannot cancel the terminal write. Bound both it and
	// the fresh authorization read, including time spent waiting for a connection.
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if err := s.repo.FinishRun(finishCtx, run.ID, result, string(failure)); err != nil {
		return dto.AIRun{}, err
	}
	visible, err := s.Get(finishCtx, actor, ticketID, run.ID)
	if err != nil {
		return dto.AIRun{}, err
	}
	if failure != "" {
		return visible, failure
	}
	return visible, nil
}

func (s *AIRuns) analyze(ctx context.Context, messages []ai.Message) (json.RawMessage, ModelFailure) {
	content, err := s.client.Complete(ctx, messages)
	if err == nil {
		var suggestion ai.Suggestion
		suggestion, err = ai.ParseSuggestion(content)
		if err == nil && ctx.Err() == nil {
			result, _ := json.Marshal(suggestion) // All fields are strings.
			return result, ""
		}
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	var upstream *ai.UpstreamError
	switch {
	case errors.Is(err, context.Canceled):
		return nil, "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return nil, "timeout"
	case errors.Is(err, ai.ErrNetwork):
		return nil, "network"
	case errors.As(err, &upstream):
		return nil, "upstream"
	case errors.Is(err, ai.ErrResponse):
		return nil, "invalid_response"
	case errors.Is(err, config.ErrAIConfig):
		return nil, "unavailable"
	default:
		return nil, "invalid_output"
	}
}
