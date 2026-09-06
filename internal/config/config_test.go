package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	inTempDir(t)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// The service must not be reachable from outside the machine unless that
	// is asked for explicitly.
	if c.Addr != "127.0.0.1:8080" {
		t.Errorf("Addr = %q, want the loopback default", c.Addr)
	}
	if c.Workers != 1 {
		t.Errorf("Workers = %d, want 1", c.Workers)
	}
	if c.AuthEnabled() {
		t.Error("authentication should be off when no keys are configured")
	}
	if c.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v, want info", c.LogLevel)
	}
	if c.LogFormat != "text" {
		t.Errorf("LogFormat = %q, want text outside production", c.LogFormat)
	}
}

func TestProductionDefaultsToJSONLogs(t *testing.T) {
	inTempDir(t)
	t.Setenv("APP_ENV", "production")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if c.LogFormat != "json" {
		t.Errorf("LogFormat = %q, want json in production", c.LogFormat)
	}
}

func TestEnvironmentOverridesDotEnv(t *testing.T) {
	dir := inTempDir(t)
	writeFile(t, filepath.Join(dir, ".env"), "LISTEN_ADDR=127.0.0.1:9999\nLOG_LEVEL=debug\n")

	// A real environment variable must win, so a stray .env on a server can
	// never override what the unit file sets.
	t.Setenv("LISTEN_ADDR", "127.0.0.1:7777")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if c.Addr != "127.0.0.1:7777" {
		t.Errorf("Addr = %q, want the environment value to win", c.Addr)
	}
	if c.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %v, want the .env value to apply where the environment is silent", c.LogLevel)
	}
}

func TestDotEnvParsing(t *testing.T) {
	dir := inTempDir(t)
	writeFile(t, filepath.Join(dir, ".env"), `
# a comment
export APP_ENV=production
API_KEYS="key-one, key-two"
CORS_ORIGINS='https://a.example,https://b.example'

READ_TIMEOUT=45s
`)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if c.Env != "production" {
		t.Errorf("Env = %q, want the export prefix to be stripped", c.Env)
	}
	if len(c.APIKeys) != 2 || c.APIKeys[0] != "key-one" || c.APIKeys[1] != "key-two" {
		t.Errorf("APIKeys = %q, want the quotes stripped and the values trimmed", c.APIKeys)
	}
	if len(c.CORSOrigins) != 2 {
		t.Errorf("CORSOrigins = %q, want two entries", c.CORSOrigins)
	}
	if c.ReadTimeout != 45*time.Second {
		t.Errorf("ReadTimeout = %v, want 45s", c.ReadTimeout)
	}
	if !c.AuthEnabled() {
		t.Error("authentication should be on once keys are configured")
	}
}

func TestInvalidValuesAreRejected(t *testing.T) {
	cases := []struct {
		name string
		key  string
		val  string
	}{
		{"unknown log level", "LOG_LEVEL", "chatty"},
		{"unknown log format", "LOG_FORMAT", "xml"},
		{"zero workers", "SWE_WORKERS", "0"},
		{"negative body limit", "MAX_BODY_BYTES", "-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inTempDir(t)
			t.Setenv(tc.key, tc.val)
			if _, err := Load(); err == nil {
				t.Errorf("%s=%s was accepted, want an error", tc.key, tc.val)
			}
		})
	}
}

// inTempDir moves the test into an empty working directory, so a .env in the
// repository cannot influence the result, and returns that directory.
func inTempDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("could not read the working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("could not enter the temporary directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	// Clear anything a previous case or the surrounding shell may have set.
	for _, key := range []string{
		"APP_ENV", "LISTEN_ADDR", "EPHE_PATH", "SWE_WORKERS", "LOG_LEVEL",
		"LOG_FORMAT", "API_KEYS", "CORS_ORIGINS", "READ_TIMEOUT",
		"WRITE_TIMEOUT", "IDLE_TIMEOUT", "SHUTDOWN_TIMEOUT", "MAX_BODY_BYTES",
	} {
		t.Setenv(key, "")
		_ = os.Unsetenv(key)
	}
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("could not write %s: %v", path, err)
	}
}
