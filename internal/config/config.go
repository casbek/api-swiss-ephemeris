// Package config loads the service settings from the environment.
//
// Settings come from environment variables. A .env file is read first as a
// convenience for development, but a variable already present in the
// environment always wins, so a systemd unit or a container definition can
// never be overridden by a stray file on disk.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds every setting the service needs.
type Config struct {
	// Env is "development" or "production". It only changes defaults such as
	// the log format; it never changes behaviour that affects results.
	Env string

	// Addr is the listen address. It defaults to loopback so the service is
	// not reachable from outside the machine unless that is asked for
	// explicitly.
	Addr string

	// EphePath is the directory holding the .se1 ephemeris files.
	EphePath string

	// Workers is the number of OS threads reserved for Swiss Ephemeris.
	Workers int

	LogLevel  slog.Level
	LogFormat string // "json" or "text"

	// APIKeys are the accepted keys. When empty, authentication is disabled
	// and every request is allowed through.
	APIKeys []string

	// CORSOrigins are the origins allowed to call the API from a browser.
	// Empty disables CORS entirely, which is right when only server side
	// clients call the service.
	CORSOrigins []string

	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration

	// MaxBodyBytes caps the size of a request body.
	MaxBodyBytes int64
}

// Load reads the configuration, applying defaults for anything not set.
func Load() (*Config, error) {
	loadDotEnv(".env")

	c := &Config{
		Env:             envString("APP_ENV", "development"),
		Addr:            envString("LISTEN_ADDR", "127.0.0.1:8080"),
		EphePath:        envString("EPHE_PATH", "./ephe"),
		LogFormat:       envString("LOG_FORMAT", ""),
		APIKeys:         envList("API_KEYS"),
		CORSOrigins:     envList("CORS_ORIGINS"),
		ReadTimeout:     envDuration("READ_TIMEOUT", 10*time.Second),
		WriteTimeout:    envDuration("WRITE_TIMEOUT", 30*time.Second),
		IdleTimeout:     envDuration("IDLE_TIMEOUT", 120*time.Second),
		ShutdownTimeout: envDuration("SHUTDOWN_TIMEOUT", 15*time.Second),
		MaxBodyBytes:    int64(envInt("MAX_BODY_BYTES", 1<<20)),
	}

	var err error
	if c.Workers, err = workers(); err != nil {
		return nil, err
	}
	if c.LogLevel, err = logLevel(envString("LOG_LEVEL", "info")); err != nil {
		return nil, err
	}
	if c.LogFormat == "" {
		// Text is easier to read while developing; production ships JSON so
		// journald and log processors can parse it.
		c.LogFormat = "text"
		if c.Env == "production" {
			c.LogFormat = "json"
		}
	}
	if c.LogFormat != "json" && c.LogFormat != "text" {
		return nil, fmt.Errorf("config: LOG_FORMAT must be json or text, got %q", c.LogFormat)
	}
	if c.MaxBodyBytes <= 0 {
		return nil, fmt.Errorf("config: MAX_BODY_BYTES must be positive, got %d", c.MaxBodyBytes)
	}
	return c, nil
}

// AuthEnabled reports whether requests must carry an API key.
func (c *Config) AuthEnabled() bool { return len(c.APIKeys) > 0 }

func workers() (int, error) {
	n := envInt("SWE_WORKERS", 1)
	if n < 1 {
		return 0, fmt.Errorf("config: SWE_WORKERS must be at least 1, got %d", n)
	}
	return n, nil
}

func logLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("config: unknown LOG_LEVEL %q", s)
	}
}

func envString(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envDuration(key string, def time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

// envList splits a comma separated variable, trimming blanks.
func envList(key string) []string {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// loadDotEnv reads simple KEY=VALUE lines from path into the environment.
//
// Variables already set are left alone. A missing file is not an error, since
// production deployments are expected to have none.
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		// Strip one layer of matching quotes.
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}
