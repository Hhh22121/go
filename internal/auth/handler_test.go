package auth_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"go-backend-template/internal/auth"
	"go-backend-template/internal/httpx/httpxtest"
	"go-backend-template/internal/user"
	"go-backend-template/internal/user/usertest"
)

type env struct {
	router *gin.Engine
	repo   *usertest.MemoryRepository
	tokens *auth.TokenService
}

func newEnv() env {
	repo := usertest.NewMemoryRepository()
	users := user.NewService(repo)
	tokens := auth.NewTokenService(strings.Repeat("s", 32), 15*time.Minute)
	r := gin.New()
	api := r.Group("/api/v1")
	auth.RegisterRoutes(api, users, tokens)
	user.RegisterRoutes(api, users, auth.RequireAuth(tokens))
	return env{router: r, repo: repo, tokens: tokens}
}

const creds = `{"email":"a@example.com","password":"password123"}` //nolint:gosec // test fixture, not a credential

type loginResp struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func login(t *testing.T, e env, body string) *loginResp {
	t.Helper()
	rec := httpxtest.Do(t, e.router, http.MethodPost, "/api/v1/auth/login", body, "")
	if rec.Code != http.StatusOK {
		return nil
	}
	r := httpxtest.DecodeJSON[loginResp](t, rec)
	return &r
}

func TestRegister(t *testing.T) {
	e := newEnv()

	rec := httpxtest.Do(t, e.router, http.MethodPost, "/api/v1/auth/register", creds, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body)
	}
	body := httpxtest.DecodeJSON[map[string]map[string]any](t, rec)
	if body["user"]["email"] != "a@example.com" {
		t.Fatalf("body = %v", body)
	}
	if _, leaked := body["user"]["password_hash"]; leaked {
		t.Fatal("password_hash leaked")
	}

	dup := httpxtest.Do(t, e.router, http.MethodPost, "/api/v1/auth/register", `{"email":"A@example.com","password":"password123"}`, "")
	if dup.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d", dup.Code)
	}

	for name, bad := range map[string]string{ //nolint:gosec // test fixtures, not credentials
		"bad email":      `{"email":"nope","password":"password123"}`,
		"short password": `{"email":"b@example.com","password":"short"}`,
		"missing fields": `{}`,
		"too many bytes": `{"email":"c@example.com","password":"` + strings.Repeat("ữ", 25) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := httpxtest.Do(t, e.router, http.MethodPost, "/api/v1/auth/register", bad, "")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestLoginAndMe(t *testing.T) {
	e := newEnv()
	if rec := httpxtest.Do(t, e.router, http.MethodPost, "/api/v1/auth/register", creds, ""); rec.Code != http.StatusCreated {
		t.Fatalf("register: %d", rec.Code)
	}

	lr := login(t, e, creds)
	if lr == nil {
		t.Fatal("login failed")
	}
	if lr.TokenType != "Bearer" || lr.ExpiresIn != 900 || lr.AccessToken == "" {
		t.Fatalf("login resp = %+v", lr)
	}

	if login(t, e, `{"email":"a@example.com","password":"wrong-password"}`) != nil {
		t.Fatal("wrong password accepted")
	}
	rec := httpxtest.Do(t, e.router, http.MethodPost, "/api/v1/auth/login", `{"email":"nobody@example.com","password":"password123"}`, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unknown email status = %d", rec.Code)
	}

	me := httpxtest.Do(t, e.router, http.MethodGet, "/api/v1/users/me", "", lr.AccessToken)
	if me.Code != http.StatusOK {
		t.Fatalf("me status = %d body = %s", me.Code, me.Body)
	}

	for name, tok := range map[string]string{"no token": "", "bad token": "abc.def.ghi"} {
		t.Run(name, func(t *testing.T) {
			rec := httpxtest.Do(t, e.router, http.MethodGet, "/api/v1/users/me", "", tok)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d", rec.Code)
			}
		})
	}

	t.Run("lowercase scheme", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
		req.Header.Set("Authorization", "bearer "+lr.AccessToken)
		rec := httptest.NewRecorder()
		e.router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("deleted user", func(t *testing.T) {
		id, err := e.tokens.Verify(lr.AccessToken)
		if err != nil {
			t.Fatal(err)
		}
		e.repo.Delete(id)
		rec := httpxtest.Do(t, e.router, http.MethodGet, "/api/v1/users/me", "", lr.AccessToken)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", rec.Code)
		}
	})
}
