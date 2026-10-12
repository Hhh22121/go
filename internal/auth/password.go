package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"github.com/Hhh22121/go-backend-template/internal/apperr"
)

const maxPasswordBytes = 72 // bcrypt limit

// HashPassword returns a bcrypt hash of pw.
func HashPassword(pw string) (string, error) {
	if len(pw) > maxPasswordBytes {
		return "", fmt.Errorf("%w: password longer than %d bytes", apperr.ErrValidation, maxPasswordBytes)
	}
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(b), nil
}

// CheckPassword reports whether pw matches hash.
func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}
