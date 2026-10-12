// Package auth issues and verifies JWT access tokens and serves the
// register/login endpoints.
package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/Hhh22121/go/internal/apperr"
)

// TokenService issues and verifies HS256 access tokens.
type TokenService struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewTokenService(secret string, ttl time.Duration) *TokenService {
	return &TokenService{secret: []byte(secret), ttl: ttl, now: time.Now}
}

func (s *TokenService) TTL() time.Duration { return s.ttl }

// Issue returns a signed token whose subject is userID.
func (s *TokenService) Issue(userID uuid.UUID) (string, error) {
	now := s.now()
	claims := jwt.RegisteredClaims{
		Subject:   userID.String(),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return tok, nil
}

// Verify checks signature, algorithm and expiry and returns the subject.
func (s *TokenService) Verify(token string) (uuid.UUID, error) {
	var claims jwt.RegisteredClaims
	_, err := jwt.ParseWithClaims(token, &claims,
		func(*jwt.Token) (any, error) { return s.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(s.now),
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: invalid token", apperr.ErrUnauthorized)
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, fmt.Errorf("%w: invalid token subject", apperr.ErrUnauthorized)
	}
	return id, nil
}
