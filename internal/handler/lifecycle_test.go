package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Planck-code/SupportFlow-AI/internal/service"
	"github.com/gin-gonic/gin"
)

func TestInvalidLifecycleRequests(t *testing.T) {
	r := gin.New()
	r.POST("/tickets/:id/resolve", Tickets{}.Lifecycle(service.Resolve))
	r.PATCH("/tickets/:id/priority", Tickets{}.Lifecycle(service.ChangePriority))
	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/tickets/0/resolve", ""},
		{"POST", "/tickets/1/resolve", `{"actor_id":1}`},
		{"POST", "/tickets/1/resolve", `{"status":"closed"}`},
		{"POST", "/tickets/1/resolve", `{"metadata":{}}`},
		{"POST", "/tickets/1/resolve", "{} {}"},
		{"PATCH", "/tickets/1/priority", `{"priority":`},
		{"PATCH", "/tickets/1/priority", `{"Priority":"high"}`},
		{"PATCH", "/tickets/1/priority", `{"priority":"high","priority":"normal"}`},
		{"PATCH", "/tickets/1/priority", `{"priority":null}`},
		{"PATCH", "/tickets/1/priority", `{"priority":"urgent"}`},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if w.Code != 400 {
			t.Errorf("%s %s: got %d", tc.method, tc.path, w.Code)
		}
	}
}
