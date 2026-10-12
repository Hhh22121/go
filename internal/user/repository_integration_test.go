//go:build integration

package user_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"go-backend-template/internal/apperr"
	"go-backend-template/internal/testutil"
	"go-backend-template/internal/user"
)

func TestPostgresRepository(t *testing.T) {
	repo := user.NewPostgresRepository(testutil.NewPostgres(t))
	ctx := context.Background()

	created, err := repo.Create(ctx, "bob@example.com", "hash")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == uuid.Nil || created.CreatedAt.IsZero() {
		t.Fatalf("defaults not populated: %+v", created)
	}

	byID, err := repo.GetByID(ctx, created.ID)
	if err != nil || byID.Email != "bob@example.com" {
		t.Fatalf("GetByID = %+v, %v", byID, err)
	}

	byEmail, err := repo.GetByEmail(ctx, "BOB@example.com") // citext
	if err != nil || byEmail.ID != created.ID {
		t.Fatalf("GetByEmail = %+v, %v", byEmail, err)
	}

	if _, err := repo.Create(ctx, "Bob@Example.com", "hash"); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("duplicate err = %v, want ErrConflict", err)
	}

	if _, err := repo.GetByID(ctx, uuid.New()); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("missing err = %v, want ErrNotFound", err)
	}
}
