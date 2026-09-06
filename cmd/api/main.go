// Command api serves the Swiss Ephemeris HTTP API.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/casbek/api-swiss-ephemeris/internal/config"
	"github.com/casbek/api-swiss-ephemeris/internal/httpapi"
	"github.com/casbek/api-swiss-ephemeris/internal/swe"
	"github.com/casbek/api-swiss-ephemeris/internal/version"
)

func main() {
	if err := run(); err != nil {
		// The logger may not exist yet when configuration fails, so report
		// to stderr and let the exit code carry the failure.
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := newLogger(cfg)
	slog.SetDefault(log)

	info := version.Get()
	log.Info("starting",
		slog.String("version", info.Version),
		slog.String("revision", info.Revision),
		slog.Bool("modified", info.Modified))

	calc, err := swe.New(swe.Config{
		EphePath: cfg.EphePath,
		Workers:  cfg.Workers,
	})
	if err != nil {
		return err
	}
	defer calc.Close()

	log.Info("ephemeris ready",
		slog.String("swiss_ephemeris", calc.Version()),
		slog.String("path", cfg.EphePath))

	// A second signal is left to the default handler, so an operator who
	// presses Ctrl+C twice can still force the process down.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return httpapi.New(cfg, log, calc).Run(ctx)
}

// newLogger writes to stdout and never to a file.
//
// Collecting and rotating logs belongs to the supervisor: journald and the
// container runtimes both cap total size, so the service cannot fill the disk
// however much it writes.
func newLogger(cfg *config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}

	var h slog.Handler
	if cfg.LogFormat == "json" {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(h)
}
