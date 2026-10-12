package auth

import (
	"errors"
	"strings"
	"testing"

	"github.com/Hhh22121/go/internal/apperr"
)

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, "correct horse") {
		t.Fatal("correct password rejected")
	}
	if CheckPassword(hash, "wrong horse") {
		t.Fatal("wrong password accepted")
	}
}

func TestHashPasswordTooLong(t *testing.T) {
	// 25 runes × 3 bytes = 75 bytes: passes a rune-count max=72 but exceeds bcrypt's byte limit.
	_, err := HashPassword(strings.Repeat("ữ", 25))
	if !errors.Is(err, apperr.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}
