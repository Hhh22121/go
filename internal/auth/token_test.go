package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/Hhh22121/go/internal/apperr"
)

var secret = strings.Repeat("k", 32)

func TestTokenRoundTrip(t *testing.T) {
	ts := NewTokenService(secret, 15*time.Minute)
	id := uuid.New()
	tok, err := ts.Issue(id)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ts.Verify(tok)
	if err != nil || got != id {
		t.Fatalf("Verify = %v, %v; want %v", got, err, id)
	}
	if ts.TTL() != 15*time.Minute {
		t.Fatalf("TTL = %v", ts.TTL())
	}
}

func TestTokenRejected(t *testing.T) {
	ts := NewTokenService(secret, 15*time.Minute)
	id := uuid.New()

	expired := NewTokenService(secret, time.Minute)
	expired.now = func() time.Time { return time.Now().Add(-time.Hour) }
	expiredTok, _ := expired.Issue(id)

	otherSecretTok, _ := NewTokenService(strings.Repeat("x", 32), time.Minute).Issue(id)

	sign := func(m jwt.SigningMethod, sub string, key any) string {
		s, err := jwt.NewWithClaims(m, jwt.RegisteredClaims{
			Subject:   sub,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		}).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	cases := map[string]string{
		"garbage":       "not-a-token",
		"expired":       expiredTok,
		"wrong secret":  otherSecretTok,
		"HS512":         sign(jwt.SigningMethodHS512, id.String(), []byte(secret)),
		"alg none":      sign(jwt.SigningMethodNone, id.String(), jwt.UnsafeAllowNoneSignatureType),
		"bad subject":   sign(jwt.SigningMethodHS256, "not-a-uuid", []byte(secret)),
		"empty subject": sign(jwt.SigningMethodHS256, "", []byte(secret)),
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ts.Verify(tok); !errors.Is(err, apperr.ErrUnauthorized) {
				t.Fatalf("err = %v, want ErrUnauthorized", err)
			}
		})
	}
}
