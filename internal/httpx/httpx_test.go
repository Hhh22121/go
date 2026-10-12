package httpx_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"go-backend-template/internal/apperr"
	"go-backend-template/internal/httpx"
	"go-backend-template/internal/httpx/httpxtest"
)

func TestWriteError(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
		msg    string
	}{
		{"not found", fmt.Errorf("user %w", apperr.ErrNotFound), http.StatusNotFound, "not_found", "user not found"},
		{"conflict", fmt.Errorf("email %w", apperr.ErrConflict), http.StatusConflict, "conflict", "email already exists"},
		{"unauthorized", apperr.ErrUnauthorized, http.StatusUnauthorized, "unauthorized", "unauthorized"},
		{"validation", fmt.Errorf("%w: bad", apperr.ErrValidation), http.StatusBadRequest, "validation_error", "invalid input: bad"},
		{"internal", errors.New("db exploded"), http.StatusInternalServerError, "internal_error", "internal server error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.GET("/", func(c *gin.Context) { httpx.WriteError(c, tc.err) })
			rec := httpxtest.Do(t, r, http.MethodGet, "/", "", "")
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d", rec.Code, tc.status)
			}
			body := httpxtest.DecodeJSON[httpxtest.ErrorBody](t, rec)
			if body.Error.Code != tc.code || body.Error.Message != tc.msg {
				t.Fatalf("body = %+v, want code %q msg %q", body.Error, tc.code, tc.msg)
			}
		})
	}
}

func TestBindJSON(t *testing.T) {
	type req struct {
		Name string `json:"name" binding:"required"`
	}
	r := gin.New()
	r.POST("/", func(c *gin.Context) {
		var in req
		if !httpx.BindJSON(c, &in) {
			return
		}
		c.JSON(http.StatusOK, in)
	})

	if rec := httpxtest.Do(t, r, http.MethodPost, "/", `{"name":"a"}`, ""); rec.Code != http.StatusOK {
		t.Fatalf("valid body: status = %d", rec.Code)
	}
	for _, body := range []string{`{}`, `not json`, ``} {
		rec := httpxtest.Do(t, r, http.MethodPost, "/", body, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %q: status = %d, want 400", body, rec.Code)
		}
		if got := httpxtest.DecodeJSON[httpxtest.ErrorBody](t, rec).Error.Code; got != "validation_error" {
			t.Fatalf("body %q: code = %q", body, got)
		}
	}
}

func TestUserID(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	if _, ok := httpx.UserID(c); ok {
		t.Fatal("UserID on empty context returned ok")
	}
	id := uuid.New()
	httpx.SetUserID(c, id)
	got, ok := httpx.UserID(c)
	if !ok || got != id {
		t.Fatalf("UserID = %v, %v; want %v, true", got, ok, id)
	}
}
