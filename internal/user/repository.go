package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Hhh22121/go-backend-template/internal/apperr"
	"github.com/Hhh22121/go-backend-template/internal/store"
)

// Repository persists users. Implementations return apperr.ErrNotFound and
// apperr.ErrConflict (wrapped) for missing rows and duplicate emails.
type Repository interface {
	Create(ctx context.Context, email, passwordHash string) (User, error)
	GetByID(ctx context.Context, id uuid.UUID) (User, error)
	GetByEmail(ctx context.Context, email string) (User, error)
}

type PostgresRepository struct {
	q *store.Queries
}

func NewPostgresRepository(db store.DBTX) *PostgresRepository {
	return &PostgresRepository{q: store.New(db)}
}

func (r *PostgresRepository) Create(ctx context.Context, email, passwordHash string) (User, error) {
	row, err := r.q.CreateUser(ctx, store.CreateUserParams{Email: email, PasswordHash: passwordHash})
	if err != nil {
		return User{}, mapErr(err)
	}
	return fromRow(row), nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (User, error) {
	row, err := r.q.GetUserByID(ctx, id)
	if err != nil {
		return User{}, mapErr(err)
	}
	return fromRow(row), nil
}

func (r *PostgresRepository) GetByEmail(ctx context.Context, email string) (User, error) {
	row, err := r.q.GetUserByEmail(ctx, email)
	if err != nil {
		return User{}, mapErr(err)
	}
	return fromRow(row), nil
}

const uniqueViolation = "23505"

func mapErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("user %w", apperr.ErrNotFound)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return fmt.Errorf("email %w", apperr.ErrConflict)
	}
	return fmt.Errorf("user repository: %w", err)
}

func fromRow(row store.User) User {
	return User{
		ID:           row.ID,
		Email:        row.Email,
		PasswordHash: row.PasswordHash,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}
