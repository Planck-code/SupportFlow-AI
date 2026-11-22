package handler

import (
	"context"
	"errors"
	"github.com/Planck-code/SupportFlow-AI/internal/dto"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

type brokenDirectory struct{}

func (brokenDirectory) ListAgents(context.Context, int, int) ([]dto.UserResponse, int64, error) {
	return nil, 0, errors.New("private DSN")
}
func TestAgentListErrors(t *testing.T) {
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("request_id", "test-id") })
	r.GET("/agents", ListAgents(brokenDirectory{}))
	for query, want := range map[string]int{"": 500, "?page=0": 400, "?page_size=101": 400, "?page_size=": 400, "?page=abc": 400, "?page=9223372036854775807&page_size=100": 400, "?page=1&page=2": 400} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/agents"+query, nil))
		if w.Code != want || strings.Contains(w.Body.String(), "private") || !strings.Contains(w.Body.String(), "test-id") {
			t.Errorf("%s: unexpected status or error body (%d)", query, w.Code)
		}
	}
}
