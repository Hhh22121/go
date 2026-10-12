// Package usertest provides an in-memory user.Repository for tests.
package usertest

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Hhh22121/go-backend-template/internal/apperr"
	"github.com/Hhh22121/go-backend-template/internal/user"
)

type MemoryRepository struct {
	mu    sync.Mutex
	users map[uuid.UUID]user.User
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{users: map[uuid.UUID]user.User{}}
}

func (r *MemoryRepository) Create(_ context.Context, email, passwordHash string) (user.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range r.users {
		if strings.EqualFold(u.Email, email) {
			return user.User{}, fmt.Errorf("email %w", apperr.ErrConflict)
		}
	}
	now := time.Now().UTC()
	u := user.User{ID: uuid.New(), Email: email, PasswordHash: passwordHash, CreatedAt: now, UpdatedAt: now}
	r.users[u.ID] = u
	return u, nil
}

func (r *MemoryRepository) GetByID(_ context.Context, id uuid.UUID) (user.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return user.User{}, fmt.Errorf("user %w", apperr.ErrNotFound)
	}
	return u, nil
}

func (r *MemoryRepository) GetByEmail(_ context.Context, email string) (user.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range r.users {
		if strings.EqualFold(u.Email, email) {
			return u, nil
		}
	}
	return user.User{}, fmt.Errorf("user %w", apperr.ErrNotFound)
}

// Delete removes a user (test-only, e.g. to simulate a deleted account).
func (r *MemoryRepository) Delete(id uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.users, id)
}
