package config

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

var validSecret = strings.Repeat("s", 32)

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(env(map[string]string{
		"DATABASE_URL": "postgres://x",
		"JWT_SECRET":   validSecret,
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := Config{
		Env:             "dev",
		HTTPAddr:        ":8080",
		DatabaseURL:     "postgres://x",
		JWTSecret:       validSecret,
		JWTTTL:          15 * time.Minute,
		ShutdownTimeout: 10 * time.Second,
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("cfg = %+v\nwant %+v", cfg, want)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := load(env(map[string]string{
		"APP_ENV":          "prod",
		"HTTP_ADDR":        ":9000",
		"DATABASE_URL":     "postgres://x",
		"JWT_SECRET":       validSecret,
		"JWT_TTL":          "1h",
		"SHUTDOWN_TIMEOUT": "3s",
		"CORS_ORIGINS":     " http://a.com , ,http://b.com",
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Env != "prod" || cfg.HTTPAddr != ":9000" || cfg.JWTTTL != time.Hour || cfg.ShutdownTimeout != 3*time.Second {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
	if want := []string{"http://a.com", "http://b.com"}; !reflect.DeepEqual(cfg.CORSOrigins, want) {
		t.Fatalf("CORSOrigins = %q, want %q", cfg.CORSOrigins, want)
	}
}

func TestLoadReportsAllErrors(t *testing.T) {
	_, err := load(env(map[string]string{
		"APP_ENV":    "staging",
		"JWT_SECRET": "short",
		"JWT_TTL":    "soon",
	}))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"APP_ENV", "DATABASE_URL", "JWT_SECRET", "JWT_TTL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}
