package user_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"go-backend-template/internal/apperr"
	"go-backend-template/internal/user"
	"go-backend-template/internal/user/usertest"
)

func TestServiceCreateNormalizesEmail(t *testing.T) {
	svc := user.NewService(usertest.NewMemoryRepository())
	u, err := svc.Create(context.Background(), "  Alice@Example.COM ", "hash")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if u.Email != "alice@example.com" {
		t.Fatalf("email = %q", u.Email)
	}
	got, err := svc.GetByEmail(context.Background(), "ALICE@example.com")
	if err != nil || got.ID != u.ID {
		t.Fatalf("GetByEmail = %v, %v", got, err)
	}
}

func TestServiceCreateDuplicate(t *testing.T) {
	svc := user.NewService(usertest.NewMemoryRepository())
	ctx := context.Background()
	if _, err := svc.Create(ctx, "a@b.com", "h"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, "A@B.com", "h"); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

func TestServiceGetByIDNotFound(t *testing.T) {
	svc := user.NewService(usertest.NewMemoryRepository())
	if _, err := svc.GetByID(context.Background(), uuid.New()); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
