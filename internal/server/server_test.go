package server_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Hhh22121/go-backend-template/internal/httpx"
	"github.com/Hhh22121/go-backend-template/internal/httpx/httpxtest"
	"github.com/Hhh22121/go-backend-template/internal/server"
)

type pinger struct{ err error }

func (p pinger) Ping(context.Context) error { return p.err }

func newEngine(db server.Pinger, origins ...string) *gin.Engine {
	return server.NewEngine(server.Options{
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		CORSOrigins: origins,
		DB:          db,
	})
}

func TestHealth(t *testing.T) {
	if rec := httpxtest.Do(t, newEngine(pinger{}), http.MethodGet, "/healthz", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("healthz = %d", rec.Code)
	}
	if rec := httpxtest.Do(t, newEngine(pinger{}), http.MethodGet, "/readyz", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("readyz ok = %d", rec.Code)
	}
	if rec := httpxtest.Do(t, newEngine(pinger{errors.New("down")}), http.MethodGet, "/readyz", "", ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz down = %d", rec.Code)
	}
}

func TestRequestID(t *testing.T) {
	e := newEngine(pinger{})

	rec := httpxtest.Do(t, e, http.MethodGet, "/healthz", "", "")
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("request id not generated")
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-ID", "abc-123")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Request-ID"); got != "abc-123" {
		t.Fatalf("request id = %q, want passthrough", got)
	}
}

func TestPanicRecovered(t *testing.T) {
	e := newEngine(pinger{})
	e.GET("/boom", func(*gin.Context) { panic("boom") })
	rec := httpxtest.Do(t, e, http.MethodGet, "/boom", "", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if code := httpxtest.DecodeJSON[httpxtest.ErrorBody](t, rec).Error.Code; code != "internal_error" {
		t.Fatalf("code = %q", code)
	}
}

func TestBodyLimit(t *testing.T) {
	e := newEngine(pinger{})
	e.POST("/echo", func(c *gin.Context) {
		var in struct {
			Name string `json:"name"`
		}
		if !httpx.BindJSON(c, &in) {
			return
		}
		c.JSON(http.StatusOK, in)
	})
	small := httpxtest.Do(t, e, http.MethodPost, "/echo", `{"name":"a"}`, "")
	if small.Code != http.StatusOK {
		t.Fatalf("small body status = %d", small.Code)
	}
	big := `{"name":"` + strings.Repeat("a", 2<<20) + `"}`
	rec := httpxtest.Do(t, e, http.MethodPost, "/echo", big, "")
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
	if code := httpxtest.DecodeJSON[httpxtest.ErrorBody](t, rec).Error.Code; code != "payload_too_large" {
		t.Fatalf("code = %q", code)
	}
}

func TestPanicStillAccessLogged(t *testing.T) {
	var buf bytes.Buffer
	e := server.NewEngine(server.Options{
		Logger: slog.New(slog.NewTextHandler(&buf, nil)),
		DB:     pinger{},
	})
	e.GET("/boom", func(*gin.Context) { panic("boom") })
	httpxtest.Do(t, e, http.MethodGet, "/boom", "", "")
	out := buf.String()
	if !strings.Contains(out, "http request") || !strings.Contains(out, "status=500") {
		t.Fatalf("access log missing for panic: %s", out)
	}
}

func TestCORS(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "http://app.local")

	rec := httptest.NewRecorder()
	newEngine(pinger{}, "http://app.local").ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://app.local" {
		t.Fatalf("allow-origin = %q", got)
	}

	rec = httptest.NewRecorder()
	newEngine(pinger{}).ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("CORS disabled but allow-origin = %q", got)
	}
}

func TestRunGracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx, "127.0.0.1:0", http.NotFoundHandler(), time.Second) }()

	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestRunListenError(t *testing.T) {
	if err := server.Run(context.Background(), "bad::addr::", http.NotFoundHandler(), time.Second); err == nil {
		t.Fatal("expected listen error")
	}
}
