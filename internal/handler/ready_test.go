package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type pingFunc func(context.Context) error

func (p pingFunc) PingContext(ctx context.Context) error { return p(ctx) }

func TestReadyz(t *testing.T) {
	for _, available := range []bool{true, false} {
		t.Run(map[bool]string{true: "available", false: "unavailable"}[available], func(t *testing.T) {
			called := false
			pinger := pingFunc(func(ctx context.Context) error {
				called = true
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("readiness ping has no deadline")
				}
				if !available {
					return errors.New("private database details")
				}
				return nil
			})
			r := gin.New()
			r.Use(func(c *gin.Context) { c.Set("request_id", "test-id"); c.Next() })
			r.GET("/readyz", Readyz(pinger))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
			want := http.StatusOK
			if !available {
				want = http.StatusServiceUnavailable
			}
			if !called || w.Code != want {
				t.Fatalf("ping called=%v, status=%d, want=%d", called, w.Code, want)
			}
			if !json.Valid(w.Body.Bytes()) || strings.Contains(w.Body.String(), "private") {
				t.Fatal("invalid or sensitive response")
			}
			if available && w.Body.String() != `{"status":"ok"}` {
				t.Fatalf("unexpected body %q", w.Body.String())
			}
			if !available && !strings.Contains(w.Body.String(), `"request_id":"test-id"`) {
				t.Fatal("missing request ID")
			}
		})
	}
}
