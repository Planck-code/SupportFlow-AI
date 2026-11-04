package auth

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const AccessTokenLifetime = 2 * time.Hour

var ErrInvalidToken = errors.New("invalid access token")

type JWT struct {
	secret []byte
	now    func() time.Time
}

func NewJWT(secret string) (*JWT, error) {
	if len(secret) < 32 {
		return nil, errors.New("JWT secret must contain at least 32 bytes")
	}
	return &JWT{secret: []byte(secret), now: time.Now}, nil
}

func (j *JWT) Issue(userID uint64) (string, error) {
	if userID == 0 {
		return "", ErrInvalidToken
	}
	now := j.now().UTC()
	claims := jwt.RegisteredClaims{
		Subject:   strconv.FormatUint(userID, 10),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenLifetime)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(j.secret)
}

func (j *JWT) Validate(raw string) (uint64, error) {
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) {
		return j.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithTimeFunc(j.now))
	if err != nil || !token.Valid || claims.IssuedAt == nil || claims.ExpiresAt == nil || !claims.ExpiresAt.After(claims.IssuedAt.Time) {
		return 0, ErrInvalidToken
	}
	id, err := strconv.ParseUint(claims.Subject, 10, 64)
	if err != nil || id == 0 || strconv.FormatUint(id, 10) != claims.Subject {
		return 0, ErrInvalidToken
	}
	return id, nil
}
