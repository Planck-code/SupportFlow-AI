package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/Planck-code/SupportFlow-AI/internal/middleware"
	"github.com/Planck-code/SupportFlow-AI/internal/service"
)

func (h Tickets) Reply(c *gin.Context) {
	id, ok := positiveID(c.Param("id"))
	if !ok {
		ticketError(c, service.ErrInvalidInput)
		return
	}
	var content string
	if err := readActionBody(c.Request.Body, "content", &content); err != nil {
		ticketError(c, err)
		return
	}
	u, _ := middleware.CurrentUser(c)
	msg, err := h.Service.Reply(c.Request.Context(), u, id, content)
	if err != nil {
		ticketError(c, err)
		return
	}
	c.JSON(201, msg)
}

func (h Tickets) Messages(c *gin.Context) {
	id, ok := positiveID(c.Param("id"))
	if !ok {
		ticketError(c, service.ErrInvalidInput)
		return
	}
	p, _, ok := ticketQuery(c)
	if !ok {
		return
	}
	u, _ := middleware.CurrentUser(c)
	items, total, err := h.Service.Messages(c.Request.Context(), u, id, p)
	paged(c, p, items, total, err)
}
