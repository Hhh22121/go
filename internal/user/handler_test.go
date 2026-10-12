package user_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"go-backend-template/internal/httpx"
	"go-backend-template/internal/httpx/httpxtest"
	"go-backend-template/internal/user"
	"go-backend-template/internal/user/usertest"
)

func newRouter(svc *user.Service, mw gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	user.RegisterRoutes(r.Group("/api/v1"), svc, mw)
	return r
}

func TestMe(t *testing.T) {
	repo := usertest.NewMemoryRepository()
	svc := user.NewService(repo)
	u, err := svc.Create(context.Background(), "me@example.com", "secret-hash")
	if err != nil {
		t.Fatal(err)
	}
	asUser := func(id uuid.UUID) gin.HandlerFunc {
		return func(c *gin.Context) { httpx.SetUserID(c, id) }
	}

	t.Run("ok", func(t *testing.T) {
		rec := httpxtest.Do(t, newRouter(svc, asUser(u.ID)), http.MethodGet, "/api/v1/users/me", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", rec.Code, rec.Body)
		}
		body := httpxtest.DecodeJSON[map[string]map[string]any](t, rec)
		if body["user"]["email"] != "me@example.com" {
			t.Fatalf("body = %v", body)
		}
		if _, leaked := body["user"]["password_hash"]; leaked {
			t.Fatal("password_hash leaked")
		}
	})

	t.Run("no user id in context", func(t *testing.T) {
		noop := func(c *gin.Context) {}
		rec := httpxtest.Do(t, newRouter(svc, noop), http.MethodGet, "/api/v1/users/me", "", "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("user deleted", func(t *testing.T) {
		ghost := uuid.New()
		rec := httpxtest.Do(t, newRouter(svc, asUser(ghost)), http.MethodGet, "/api/v1/users/me", "", "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", rec.Code)
		}
	})
}
