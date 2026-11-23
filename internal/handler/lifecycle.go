package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Planck-code/SupportFlow-AI/internal/middleware"
	"github.com/Planck-code/SupportFlow-AI/internal/service"
)

func (h Tickets) Lifecycle(action service.TicketAction) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := positiveID(c.Param("id"))
		if !ok {
			ticketError(c, service.ErrInvalidInput)
			return
		}
		var field, priority string
		if action == service.ChangePriority {
			field = "priority"
		}
		if err := readActionBody(c.Request.Body, field, &priority); err != nil {
			ticketError(c, err)
			return
		}
		user, _ := middleware.CurrentUser(c)
		ticket, err := h.Service.ApplyAction(c.Request.Context(), user.ID, id, action, priority)
		if err != nil {
			ticketError(c, err)
			return
		}
		c.JSON(http.StatusOK, ticket)
	}
}
