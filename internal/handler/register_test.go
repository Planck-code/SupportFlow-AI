package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Planck-code/SupportFlow-AI/internal/model"
	"github.com/Planck-code/SupportFlow-AI/internal/service"
)

type registrationUsers struct {
	calls int
	err   error
}

func (r *registrationUsers) Create(_ context.Context, u *model.User) error {
	r.calls++
	u.ID = 1
	u.CreatedAt = time.Now().UTC()
	return r.err
}

func TestRegistrationHTTP(t *testing.T) {
	valid := map[string]any{"email": "  TeSt@Example.COM ", "password": "12345678", "display_name": " Test User "}
	for _, tt := range []struct {
		name       string
		change     func(map[string]any)
		storageErr error
		want       int
	}{
		{name: "success", want: 201},
		{name: "missing email", change: func(m map[string]any) { delete(m, "email") }, want: 400},
		{name: "missing password", change: func(m map[string]any) { delete(m, "password") }, want: 400},
		{name: "missing display", change: func(m map[string]any) { delete(m, "display_name") }, want: 400},
		{name: "invalid email", change: func(m map[string]any) { m["email"] = "bad" }, want: 400},
		{name: "blank display", change: func(m map[string]any) { m["display_name"] = "   " }, want: 400},
		{name: "long display", change: func(m map[string]any) { m["display_name"] = strings.Repeat("名", 65) }, want: 400},
		{name: "short password", change: func(m map[string]any) { m["password"] = "short" }, want: 400},
		{name: "72 bytes", change: func(m map[string]any) { m["password"] = strings.Repeat("a", 72) }, want: 201},
		{name: "73 bytes", change: func(m map[string]any) { m["password"] = strings.Repeat("a", 73) }, want: 400},
		{name: "role admin", change: func(m map[string]any) { m["role"] = "admin" }, want: 400},
		{name: "role null", change: func(m map[string]any) { m["role"] = nil }, want: 400},
		{name: "duplicate", storageErr: service.ErrEmailExists, want: 409},
		{name: "internal", storageErr: errors.New("private SQL password_hash DSN"), want: 500},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := map[string]any{}
			for k, v := range valid {
				body[k] = v
			}
			if tt.change != nil {
				tt.change(body)
			}
			data, _ := json.Marshal(body)
			users := &registrationUsers{err: tt.storageErr}
			r := gin.New()
			r.Use(func(c *gin.Context) { c.Set("request_id", "test-id"); c.Next() })
			r.POST("/api/v1/auth/register", RegisterUser(service.NewRegistration(users)))
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(data))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			if w.Code != tt.want {
				t.Fatalf("status=%d want=%d", w.Code, tt.want)
			}
			if !json.Valid(w.Body.Bytes()) {
				t.Fatal("invalid JSON response")
			}
			var response map[string]json.RawMessage
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if _, ok := response["password"]; ok {
				t.Fatal("password field leaked")
			}
			if _, ok := response["password_hash"]; ok {
				t.Fatal("hash field leaked")
			}
			if strings.Contains(w.Body.String(), "private SQL") || strings.Contains(w.Body.String(), "12345678") {
				t.Fatal("sensitive input or error leaked")
			}
			if tt.want == 400 && users.calls != 0 {
				t.Fatal("invalid request reached repository")
			}
			if tt.want == 201 {
				if len(response) != 5 || string(response["email"]) != `"test@example.com"` || string(response["display_name"]) != `"Test User"` || string(response["role"]) != `"user"` {
					t.Fatal("response DTO incorrect")
				}
			} else if string(response["request_id"]) != `"test-id"` {
				t.Fatal("missing error request ID")
			}
		})
	}
}

func TestRegistrationMalformedJSON(t *testing.T) {
	for _, body := range []string{`{`, `null`, `[]`, `{"email":1,"password":"12345678","display_name":"Test"}`} {
		users := &registrationUsers{}
		r := gin.New()
		r.POST("/register", RegisterUser(service.NewRegistration(users)))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("POST", "/register", strings.NewReader(body)))
		if w.Code != 400 || users.calls != 0 {
			t.Fatal("malformed JSON accepted")
		}
	}
}
