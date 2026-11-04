package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWT(t *testing.T) {
	secret := strings.Repeat("test-only-", 4)
	tokens, err := NewJWT(secret)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	tokens.now = func() time.Time { return now }
	raw, err := tokens.Issue(42)
	if err != nil {
		t.Fatal(err)
	}
	if id, err := tokens.Validate(raw); err != nil || id != 42 {
		t.Fatal("issued token rejected")
	}
	parsed, err := jwt.Parse(raw, func(*jwt.Token) (any, error) { return []byte(secret), nil }, jwt.WithTimeFunc(tokens.now))
	if err != nil {
		t.Fatal(err)
	}
	claims := parsed.Claims.(jwt.MapClaims)
	if len(claims) != 3 || claims["sub"] != "42" || claims["iat"] != float64(now.Unix()) || claims["exp"] != float64(now.Add(2*time.Hour).Unix()) {
		t.Fatal("incorrect claims or lifetime")
	}
	if _, err := tokens.Issue(0); err == nil {
		t.Fatal("zero user ID accepted")
	}
	if _, err := NewJWT("short"); err == nil {
		t.Fatal("weak secret accepted")
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		method jwt.SigningMethod
		key    any
		change func(jwt.MapClaims)
	}{
		{name: "expired", change: func(c jwt.MapClaims) {
			c["iat"] = now.Add(-3 * time.Hour).Unix()
			c["exp"] = now.Add(-time.Second).Unix()
		}},
		{name: "expiry boundary", change: func(c jwt.MapClaims) { c["exp"] = now.Unix() }},
		{name: "missing exp", change: func(c jwt.MapClaims) { delete(c, "exp") }},
		{name: "missing iat", change: func(c jwt.MapClaims) { delete(c, "iat") }},
		{name: "future iat", change: func(c jwt.MapClaims) { c["iat"] = now.Add(time.Minute).Unix() }},
		{name: "missing sub", change: func(c jwt.MapClaims) { delete(c, "sub") }},
		{name: "zero sub", change: func(c jwt.MapClaims) { c["sub"] = "0" }},
		{name: "negative sub", change: func(c jwt.MapClaims) { c["sub"] = "-1" }},
		{name: "email sub", change: func(c jwt.MapClaims) { c["sub"] = "test@example.com" }},
		{name: "overflow sub", change: func(c jwt.MapClaims) { c["sub"] = "18446744073709551616" }},
		{name: "numeric sub", change: func(c jwt.MapClaims) { c["sub"] = 42 }},
		{name: "HS384", method: jwt.SigningMethodHS384},
		{name: "none", method: jwt.SigningMethodNone, key: jwt.UnsafeAllowNoneSignatureType},
		{name: "RS256", method: jwt.SigningMethodRS256, key: key},
		{name: "wrong signature", key: []byte(strings.Repeat("other", 8))},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := jwt.MapClaims{"sub": "42", "iat": now.Unix(), "exp": now.Add(2 * time.Hour).Unix()}
			if tt.change != nil {
				tt.change(c)
			}
			method := tt.method
			if method == nil {
				method = jwt.SigningMethodHS256
			}
			signingKey := tt.key
			if signingKey == nil {
				signingKey = []byte(secret)
			}
			signed, err := jwt.NewWithClaims(method, c).SignedString(signingKey)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tokens.Validate(signed); err == nil {
				t.Fatal("invalid token accepted")
			}
		})
	}
	tokens.now = func() time.Time { return now.Add(2 * time.Hour) }
	if _, err := tokens.Validate(raw); err == nil {
		t.Fatal("expired issued token accepted")
	}
}
