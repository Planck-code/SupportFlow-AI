package handler

import (
	"encoding/json"
	"io"

	"github.com/gin-gonic/gin"

	"github.com/Planck-code/SupportFlow-AI/internal/middleware"
	"github.com/Planck-code/SupportFlow-AI/internal/service"
)

// Read exactly one object with one case-sensitive string field.
func messageContent(body io.Reader) (string, error) {
	d := json.NewDecoder(body)
	open, err := d.Token()
	if err != nil || open != json.Delim('{') {
		return "", service.ErrInvalidInput
	}
	key, err := d.Token()
	var content string
	if err != nil || key != "content" || d.Decode(&content) != nil {
		return "", service.ErrInvalidInput
	}
	end, err := d.Token()
	if err != nil || end != json.Delim('}') {
		return "", service.ErrInvalidInput
	}
	if _, err := d.Token(); err != io.EOF {
		return "", service.ErrInvalidInput
	}
	return content, nil
}

func (h Tickets) Reply(c *gin.Context) {
	id, ok := positiveID(c.Param("id"))
	if !ok {
		ticketError(c, service.ErrInvalidInput)
		return
	}
	content, err := messageContent(c.Request.Body)
	if err != nil {
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
