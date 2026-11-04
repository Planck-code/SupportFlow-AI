package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Planck-code/SupportFlow-AI/internal/auth"
	"github.com/Planck-code/SupportFlow-AI/internal/dto"
	"github.com/Planck-code/SupportFlow-AI/internal/middleware"
	"github.com/Planck-code/SupportFlow-AI/internal/model"
	"github.com/Planck-code/SupportFlow-AI/internal/repository"
	"github.com/Planck-code/SupportFlow-AI/internal/service"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type authUsers struct {
	user model.User
	err  error
}

func (u *authUsers) FindByEmail(ctx context.Context, email string) (model.User, error) {
	if err := ctx.Err(); err != nil {
		return model.User{}, err
	}
	if u.err != nil {
		return model.User{}, u.err
	}
	if email != u.user.Email {
		return model.User{}, repository.ErrUserNotFound
	}
	return u.user, nil
}
func (u *authUsers) FindByID(ctx context.Context, id uint64) (model.User, error) {
	if err := ctx.Err(); err != nil {
		return model.User{}, err
	}
	if u.err != nil {
		return model.User{}, u.err
	}
	if id != u.user.ID {
		return model.User{}, repository.ErrUserNotFound
	}
	return u.user, nil
}

func TestLoginAndAuthenticationHTTP(t *testing.T) {
	password := "  test-password  "
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	users := &authUsers{user: model.User{ID: 7, Email: "test@example.com", PasswordHash: string(hash), DisplayName: "Test", Role: "user", CreatedAt: time.Now().UTC()}}
	secret := strings.Repeat("unit-test-only-", 3)
	tokens, err := auth.NewJWT(secret)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	login, err := service.NewLogin(users, tokens)
	if err != nil {
		t.Fatal(err)
	}
	r := New(slog.New(slog.NewJSONHandler(&logs, nil)), pingFunc(func(context.Context) error { return nil }), nil, login, middleware.Authenticate(tokens, users))
	request := func(method, path, body, header string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		r.ServeHTTP(w, req)
		if strings.Contains(w.Body.String(), "password") || strings.Contains(w.Body.String(), "private-db") {
			t.Fatal("sensitive response")
		}
		if w.Code >= 400 {
			var e map[string]string
			if json.Unmarshal(w.Body.Bytes(), &e) != nil || e["request_id"] == "" || e["request_id"] != w.Header().Get("X-Request-ID") {
				t.Fatal("incorrect error envelope")
			}
		}
		return w
	}
	var token string
	var credentialFailure string
	for _, tt := range []struct {
		name, email, password string
		want                  int
		dbErr                 error
	}{
		{name: "success normalized", email: "  TEST@Example.COM ", password: password, want: 200},
		{name: "wrong password", email: "test@example.com", password: "incorrect", want: 401},
		{name: "missing account", email: "missing@example.com", password: "incorrect", want: 401},
		{name: "short wrong password", email: "test@example.com", password: "x", want: 401},
		{name: "overlong password", email: "test@example.com", password: strings.Repeat("x", 73), want: 401},
		{name: "missing email", password: password, want: 400},
		{name: "missing password", email: "test@example.com", want: 400},
		{name: "invalid email", email: "bad", password: password, want: 400},
		{name: "database failure", email: "test@example.com", password: password, want: 500, dbErr: errors.New("private-db DSN")},
		{name: "corrupt hash", email: "test@example.com", password: password, want: 500},
	} {
		t.Run(tt.name, func(t *testing.T) {
			users.err = tt.dbErr
			users.user.PasswordHash = string(hash)
			if tt.name == "corrupt hash" {
				users.user.PasswordHash = "broken"
			}
			body, _ := json.Marshal(map[string]string{"email": tt.email, "password": tt.password, "role": "admin", "id": "999"})
			w := request("POST", "/api/v1/auth/login", string(body), "")
			if w.Code != tt.want {
				t.Fatalf("status=%d want=%d", w.Code, tt.want)
			}
			if tt.want == 200 {
				var result dto.LoginResponse
				if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.TokenType != "Bearer" || result.ExpiresIn != 7200 {
					t.Fatal("incorrect token response")
				}
				id, err := tokens.Validate(result.AccessToken)
				if err != nil || id != 7 {
					t.Fatal("invalid login token")
				}
				token = result.AccessToken
			}
			if tt.want == 401 {
				var e map[string]string
				json.Unmarshal(w.Body.Bytes(), &e)
				if credentialFailure == "" {
					credentialFailure = e["message"]
				}
				if e["message"] != credentialFailure {
					t.Fatal("credentials reveal account existence")
				}
			}
		})
	}
	users.err = nil
	users.user.PasswordHash = string(hash)
	longPassword := strings.Repeat("a", 72)
	longHash, err := bcrypt.GenerateFromPassword([]byte(longPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	users.user.PasswordHash = string(longHash)
	for _, input := range []struct {
		password string
		want     int
	}{{longPassword, 200}, {longPassword + "b", 401}} {
		body, _ := json.Marshal(map[string]string{"email": "test@example.com", "password": input.password})
		if w := request("POST", "/api/v1/auth/login", string(body), ""); w.Code != input.want {
			t.Fatal("bcrypt byte boundary not enforced")
		}
	}
	users.user.PasswordHash = string(hash)
	for _, body := range []string{`{`, `null`, `[]`, `{"email":3,"password":"test"}`} {
		if w := request("POST", "/api/v1/auth/login", body, ""); w.Code != 400 {
			t.Fatal("malformed login accepted")
		}
	}
	sign := func(method jwt.SigningMethod, claims jwt.MapClaims, key any) string {
		t.Helper()
		raw, err := jwt.NewWithClaims(method, claims).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	claims := func(sub string) jwt.MapClaims {
		return jwt.MapClaims{"sub": sub, "iat": time.Now().Add(-time.Minute).Unix(), "exp": time.Now().Add(time.Hour).Unix(), "role": "admin"}
	}
	expired := claims("7")
	expired["iat"] = time.Now().Add(-3 * time.Hour).Unix()
	expired["exp"] = time.Now().Add(-time.Minute).Unix()
	for _, header := range []string{"", "Bearer", "Basic " + token, "Bearer " + token + " extra", "Bearer invalid", "Bearer " + sign(jwt.SigningMethodHS256, claims("7"), []byte(strings.Repeat("wrong", 8))), "Bearer " + sign(jwt.SigningMethodHS256, expired, []byte(secret)), "Bearer " + sign(jwt.SigningMethodHS384, claims("7"), []byte(secret)), "Bearer " + sign(jwt.SigningMethodNone, claims("7"), jwt.UnsafeAllowNoneSignatureType), "Bearer " + sign(jwt.SigningMethodHS256, claims("999"), []byte(secret))} {
		if w := request("GET", "/api/v1/users/me", "", header); w.Code != 401 {
			t.Fatalf("invalid auth status=%d", w.Code)
		}
	}
	// Current DB fields win over signed role claims and request-supplied IDs.
	users.user.DisplayName = "Updated"
	users.user.Role = "agent"
	for _, header := range []string{"Bearer " + token, "bearer " + sign(jwt.SigningMethodHS256, claims("7"), []byte(secret))} {
		w := request("GET", "/api/v1/users/me?id=999&role=admin", "", header)
		var user dto.UserResponse
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &user) != nil || user.ID != 7 || user.Role != "agent" || user.DisplayName != "Updated" {
			t.Fatal("me did not use current database user")
		}
		var fields map[string]any
		json.Unmarshal(w.Body.Bytes(), &fields)
		if len(fields) != 5 {
			t.Fatal("unsafe user DTO")
		}
	}
	users.err = errors.New("private-db DSN")
	if w := request("GET", "/api/v1/users/me", "", "Bearer "+token); w.Code != 500 {
		t.Fatal("database fault misclassified")
	}
	if strings.Contains(logs.String(), token) || strings.Contains(logs.String(), password) || strings.Contains(logs.String(), string(hash)) || strings.Contains(logs.String(), "private-db") {
		t.Fatal("credentials leaked into logs")
	}
	duplicateHeader := httptest.NewRequest("GET", "/api/v1/users/me", nil)
	duplicateHeader.Header.Add("Authorization", "Bearer "+token)
	duplicateHeader.Header.Add("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, duplicateHeader)
	if w.Code != 401 {
		t.Fatal("ambiguous authorization headers accepted")
	}
}
