package handler

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/url"
	"slices"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Planck-code/SupportFlow-AI/internal/dto"
	"github.com/Planck-code/SupportFlow-AI/internal/middleware"
	"github.com/Planck-code/SupportFlow-AI/internal/service"
)

type Tickets struct{ Service *service.TicketService }

func ticketError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrAssignmentConflict), errors.Is(err, service.ErrReplyConflict):
		writeError(c, 409, "ticket_conflict", "ticket action unavailable")
	case errors.Is(err, service.ErrInvalidAssignee):
		writeError(c, 400, "invalid_assignee", "assignee must be an existing agent")
	case errors.Is(err, service.ErrInvalidInput):
		writeError(c, 400, "invalid_input", "invalid ticket input or query")
	case errors.Is(err, service.ErrTicketForbidden):
		writeError(c, 403, "forbidden", "insufficient permissions")
	case errors.Is(err, gorm.ErrRecordNotFound):
		writeError(c, 404, "not_found", "ticket not found")
	default:
		writeError(c, 500, "internal_error", "internal server error")
	}
}

// A claim takes no fields; assignment takes exactly one positive integer.
func assignmentBody(body io.Reader, claim bool) (uint64, bool) {
	d := json.NewDecoder(body)
	first, err := d.Token()
	if claim && err == io.EOF {
		return 0, true
	}
	if err != nil || first != json.Delim('{') {
		return 0, false
	}
	var id uint64
	if !claim {
		key, err := d.Token()
		if err != nil || key != "assignee_id" || d.Decode(&id) != nil || id == 0 {
			return 0, false
		}
	}
	end, err := d.Token()
	if err != nil || end != json.Delim('}') {
		return 0, false
	}
	if _, err = d.Token(); err != io.EOF {
		return 0, false
	}
	return id, true
}

func (h Tickets) Claim(c *gin.Context)  { h.changeAssignment(c, true) }
func (h Tickets) Assign(c *gin.Context) { h.changeAssignment(c, false) }

func (h Tickets) changeAssignment(c *gin.Context, claim bool) {
	id, ok := positiveID(c.Param("id"))
	if !ok {
		ticketError(c, service.ErrInvalidInput)
		return
	}
	target, ok := assignmentBody(c.Request.Body, claim)
	if !ok {
		ticketError(c, service.ErrInvalidInput)
		return
	}
	actor, _ := middleware.CurrentUser(c)
	var ticket dto.TicketDetail
	var err error
	if claim {
		ticket, err = h.Service.Claim(c.Request.Context(), actor, id)
	} else {
		ticket, err = h.Service.Assign(c.Request.Context(), actor, id, target)
	}
	if err != nil {
		ticketError(c, err)
		return
	}
	c.JSON(200, ticket)
}

func (h Tickets) Create(c *gin.Context) {
	defaultCategory := "general"
	in := dto.CreateTicket{Category: &defaultCategory}
	if err := c.ShouldBindJSON(&in); err != nil {
		ticketError(c, service.ErrInvalidInput)
		return
	}
	user, _ := middleware.CurrentUser(c)
	ticket, err := h.Service.Create(c.Request.Context(), user, in)
	if err != nil {
		ticketError(c, err)
		return
	}
	c.JSON(201, ticket)
}

func ticketQuery(c *gin.Context, allowed ...string) (dto.Page, url.Values, bool) {
	q, err := url.ParseQuery(c.Request.URL.RawQuery)
	n, nOK := positiveParam(q, "page", 1)
	size, sizeOK := positiveParam(q, "page_size", 20)
	valid := err == nil && nOK && sizeOK && size <= 100 && n-1 <= math.MaxInt/size
	for k, v := range q {
		if len(v) != 1 || v[0] == "" || (k != "page" && k != "page_size" && !slices.Contains(allowed, k)) {
			valid = false
			break
		}
	}
	if !valid {
		ticketError(c, service.ErrInvalidInput)
	}
	return dto.Page{Number: n, Size: size}, q, valid
}

func paged(c *gin.Context, p dto.Page, items any, total int64, err error) {
	if err != nil {
		ticketError(c, err)
		return
	}
	c.JSON(200, gin.H{"page": p.Number, "page_size": p.Size, "items": items, "total": total})
}

func positiveID(s string) (uint64, bool) {
	id, err := strconv.ParseUint(s, 10, 64)
	return id, err == nil && id != 0
}

func (h Tickets) List(c *gin.Context) {
	p, q, ok := ticketQuery(c, "status", "category", "unassigned", "assignee_id")
	if !ok {
		return
	}
	f := dto.TicketFilter{Category: q.Get("category"), Status: q.Get("status")}
	if q.Has("unassigned") {
		if q.Get("unassigned") != "true" {
			ticketError(c, service.ErrInvalidInput)
			return
		}
		f.Unassigned = true
	}
	if q.Has("assignee_id") {
		f.AssigneeID, ok = positiveID(q.Get("assignee_id"))
		if !ok {
			ticketError(c, service.ErrInvalidInput)
			return
		}
	}
	u, _ := middleware.CurrentUser(c)
	items, total, err := h.Service.List(c.Request.Context(), u, f, p)
	paged(c, p, items, total, err)
}

func (h Tickets) Queue(c *gin.Context) {
	p, q, ok := ticketQuery(c, "category")
	if !ok {
		return
	}
	u, _ := middleware.CurrentUser(c)
	items, total, err := h.Service.Queue(c.Request.Context(), u, q.Get("category"), p)
	paged(c, p, items, total, err)
}

func (h Tickets) Detail(c *gin.Context) {
	id, ok := positiveID(c.Param("id"))
	if !ok {
		ticketError(c, service.ErrInvalidInput)
		return
	}
	u, _ := middleware.CurrentUser(c)
	ticket, err := h.Service.Detail(c.Request.Context(), u, id)
	if err != nil {
		ticketError(c, err)
		return
	}
	c.JSON(200, ticket)
}

func (h Tickets) Events(c *gin.Context) {
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
	items, total, err := h.Service.Events(c.Request.Context(), u, id, p)
	paged(c, p, items, total, err)
}
