package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestReplyRejectsInvalidRequest(t *testing.T) {
	r := gin.New()
	// Invalid input must be rejected before any database access.
	r.POST("/tickets/:id/messages", Tickets{}.Reply)
	for _, body := range []string{`{}`, `{"content":`, `{"Content":"x"}`, `{"content":"x","author_id":2}`,
		`{"content":"x","ticket_id":1}`, `{"content":7}`, `{"content":null}`, `{"content":"x"} {}`,
		`{"content":"a","content":"b"}`, `{"content":" \t "}`, `{"content":""}`} {
		res := httptest.NewRecorder()
		r.ServeHTTP(res, httptest.NewRequest("POST", "/tickets/1/messages", strings.NewReader(body)))
		if res.Code != 400 {
			t.Fatalf("expected bad request, got %d", res.Code)
		}
	}
}
