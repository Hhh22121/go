//go:build integration

package database_test

import (
	"context"
	"testing"

	"github.com/Hhh22121/go/internal/database"
	"github.com/Hhh22121/go/internal/testutil"
)

func TestMigrateIsIdempotent(t *testing.T) {
	pool := testutil.NewPostgres(t) // already migrated once
	ctx := context.Background()

	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&n); err != nil {
		t.Fatalf("users table missing: %v", err)
	}
}
