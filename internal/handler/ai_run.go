package handler

import (
	"errors"
	"fmt"

	"github.com/Planck-code/SupportFlow-AI/internal/middleware"
	"github.com/Planck-code/SupportFlow-AI/internal/service"
	"github.com/gin-gonic/gin"
)

func (h Tickets) StartAI(c *gin.Context) {
	id, ok := positiveID(c.Param("id"))
	if !ok || readActionBody(c.Request.Body, "", nil) != nil {
		ticketError(c, service.ErrInvalidInput)
		return
	}
	user, _ := middleware.CurrentUser(c)
	run, err := h.AI.Start(c.Request.Context(), user.ID, id)
	if run.ID != 0 {
		c.Header("Location", fmt.Sprintf("/api/v1/tickets/%d/ai-runs/%d", id, run.ID))
	}
	var failure service.ModelFailure
	switch {
	case err == nil:
		c.JSON(201, run)
	case errors.Is(err, service.ErrAIUnavailable):
		writeError(c, 503, "ai_unavailable", "AI service unavailable")
	case errors.As(err, &failure):
		switch failure {
		case "timeout":
			writeError(c, 504, "ai_timeout", "AI analysis timed out")
		case "unavailable", "canceled":
			writeError(c, 503, "ai_unavailable", "AI analysis unavailable")
		default:
			writeError(c, 502, "ai_analysis_failed", "AI analysis failed")
		}
	default:
		ticketError(c, err)
	}
}

func (h Tickets) GetAI(c *gin.Context) {
	id, ok := positiveID(c.Param("id"))
	runID, valid := positiveID(c.Param("run_id"))
	if !ok || !valid {
		ticketError(c, service.ErrInvalidInput)
		return
	}
	user, _ := middleware.CurrentUser(c)
	run, err := h.AI.Get(c.Request.Context(), user.ID, id, runID)
	if err != nil {
		ticketError(c, err)
		return
	}
	c.JSON(200, run)
}
