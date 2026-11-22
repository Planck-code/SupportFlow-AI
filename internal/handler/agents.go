package handler

import (
	"context"
	"math"
	"net/url"
	"strconv"

	"github.com/Planck-code/SupportFlow-AI/internal/dto"
	"github.com/gin-gonic/gin"
)

type AgentDirectory interface {
	ListAgents(ctx context.Context, offset, limit int) ([]dto.UserResponse, int64, error)
}

func ListAgents(directory AgentDirectory) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, err := url.ParseQuery(c.Request.URL.RawQuery)
		page, validPage := positiveParam(query, "page", 1)
		size, validSize := positiveParam(query, "page_size", 20)
		if err != nil || !validPage || !validSize || size > 100 || page-1 > math.MaxInt/size {
			writeError(c, 400, "invalid_input", "invalid page or page_size")
			return
		}
		items, total, err := directory.ListAgents(c.Request.Context(), (page-1)*size, size)
		if err != nil {
			writeError(c, 500, "internal_error", "internal server error")
			return
		}
		c.JSON(200, gin.H{"items": items, "total": total, "page": page, "page_size": size})
	}
}

func positiveParam(query url.Values, key string, fallback int) (int, bool) {
	values, ok := query[key]
	if !ok {
		return fallback, true
	}
	if len(values) != 1 {
		return 0, false
	}
	n, err := strconv.Atoi(values[0])
	return n, err == nil && n > 0
}
