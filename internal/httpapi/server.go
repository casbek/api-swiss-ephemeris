// Package httpapi serves the HTTP interface of the service.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/casbek/api-swiss-ephemeris/internal/astro"
	"github.com/casbek/api-swiss-ephemeris/internal/config"
	"github.com/casbek/api-swiss-ephemeris/internal/swe"
)

// healthPath is exempt from authentication and from the access log: a load
// balancer probes it every few seconds and those lines would drown out the
// real traffic.
const healthPath = "/health"

// Server holds everything the handlers need.
type Server struct {
	cfg    *config.Config
	log    *slog.Logger
	calc   *swe.Calculator
	engine *astro.Engine
	http   *http.Server

	startedAt time.Time

	// draining is set while the server is shutting down, so the readiness
	// probe can report that no new work should be sent here.
	draining atomic.Bool

	// boundAddr is the address actually being listened on, which differs
	// from the configured one when port 0 asks the system to choose.
	boundAddr atomic.Pointer[net.Addr]
	// listening is closed once the socket is open and Addr is set.
	listening chan struct{}
}

// New builds the server and its routes.
func New(cfg *config.Config, log *slog.Logger, calc *swe.Calculator) *Server {
	s := &Server{
		cfg:       cfg,
		log:       log,
		calc:      calc,
		engine:    astro.NewEngine(calc),
		startedAt: time.Now(),
		listening: make(chan struct{}),
	}

	handler := s.routes()

	// Listed outermost first. RequestID comes before the log so every line
	// carries an id, and Recover sits inside them so a panic is still
	// logged and still gets an id in its response.
	mw := []func(http.Handler) http.Handler{
		RequestID,
		AccessLog(log, healthPath),
		Recover(log),
		CORS(cfg.CORSOrigins),
		MaxBody(cfg.MaxBodyBytes),
	}
	if cfg.AuthEnabled() {
		mw = append(mw, APIKeyAuth(cfg.APIKeys, healthPath, "/v1/license"))
	}

	s.http = &http.Server{
		Addr:              cfg.Addr,
		Handler:           chain(handler, mw...),
		ReadTimeout:       cfg.ReadTimeout,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	return s
}

// routes registers every path the service answers.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET "+healthPath, s.handleHealth)
	mux.HandleFunc("GET /v1", s.handleIndex)
	mux.HandleFunc("GET /v1/license", s.handleLicense)
	mux.HandleFunc("GET /v1/reference/{topic}", s.handleReference)
	mux.HandleFunc("POST /v1/time", s.handleTime)
	mux.HandleFunc("POST /v1/natal", s.handleNatal)
	mux.HandleFunc("POST /v1/transits", s.handleTransits)
	mux.HandleFunc("POST /v1/synastry", s.handleSynastry)
	mux.HandleFunc("POST /v1/composite", s.handleComposite)
	mux.HandleFunc("POST /v1/progressions", s.handleProgressions)
	mux.HandleFunc("POST /v1/returns", s.handleReturns)
	mux.HandleFunc("POST /v1/ephemeris", s.handleEphemeris)
	mux.HandleFunc("POST /v1/moon/phases", s.handleMoonPhases)
	mux.HandleFunc("POST /v1/retrogrades", s.handleRetrogrades)
	mux.HandleFunc("POST /v1/eclipses", s.handleEclipses)
	mux.HandleFunc("POST /v1/rise-set", s.handleRiseSet)

	// Anything else is a 404 in problem+json rather than the plain text
	// ServeMux would produce.
	mux.HandleFunc("/", s.handleNotFound)

	return mux
}

// Run serves until the context is cancelled, then shuts down gracefully.
func (s *Server) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return fmt.Errorf("httpapi: could not listen on %s: %w", s.cfg.Addr, err)
	}

	addr := listener.Addr()
	s.boundAddr.Store(&addr)
	close(s.listening)

	s.log.Info("server listening",
		slog.String("addr", listener.Addr().String()),
		slog.String("env", s.cfg.Env),
		slog.Bool("auth_enabled", s.cfg.AuthEnabled()),
		slog.Int("swe_workers", s.cfg.Workers))

	serveErr := make(chan error, 1)
	go func() {
		if err := s.http.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}

	// Report unready before closing, so a load balancer can take this
	// instance out of rotation while in-flight requests finish.
	s.draining.Store(true)
	s.log.Info("shutting down", slog.Duration("grace", s.cfg.ShutdownTimeout))

	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
	defer cancel()

	if err := s.http.Shutdown(shutdownCtx); err != nil {
		// Requests still running when the grace period expired were cut
		// off. Say so plainly rather than reporting a clean stop.
		return fmt.Errorf("httpapi: shutdown did not complete within %s: %w", s.cfg.ShutdownTimeout, err)
	}
	s.log.Info("stopped cleanly")
	return <-serveErr
}

// Handler exposes the routed handler.
func (s *Server) Handler() http.Handler { return s.http.Handler }

// Listening is closed once the socket is open. Wait on it before reading Addr.
func (s *Server) Listening() <-chan struct{} { return s.listening }

// Addr reports the address being served. It is only meaningful after
// Listening is closed, and it is the way to discover the port when the
// configured address ends in :0.
func (s *Server) Addr() net.Addr {
	if a := s.boundAddr.Load(); a != nil {
		return *a
	}
	return nil
}
