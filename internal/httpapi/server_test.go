package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/casbek/api-swiss-ephemeris/internal/config"
	"github.com/casbek/api-swiss-ephemeris/internal/swe"
)

// newTestServer builds a server backed by a real calculator, so the tests
// exercise the same path production does.
func newTestServer(t *testing.T, adjust func(*config.Config)) *Server {
	t.Helper()

	calc, err := swe.New(swe.Config{EphePath: "../../ephe"})
	if err != nil {
		t.Fatalf("could not start calculator: %v", err)
	}
	t.Cleanup(calc.Close)

	cfg := &config.Config{
		Env:             "test",
		Addr:            "127.0.0.1:0",
		EphePath:        "../../ephe",
		Workers:         1,
		LogLevel:        slog.LevelError,
		LogFormat:       "text",
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    5 * time.Second,
		IdleTimeout:     30 * time.Second,
		ShutdownTimeout: 5 * time.Second,
		MaxBodyBytes:    1 << 20,
	}
	if adjust != nil {
		adjust(cfg)
	}

	// Discard log output so a failing test shows only its own assertions.
	log := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	return New(cfg, log, calc)
}

func do(t *testing.T, s *Server, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestHealthReportsReady(t *testing.T) {
	s := newTestServer(t, nil)
	rec := do(t, s, http.MethodGet, "/health", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var body healthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("could not decode body: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Status)
	}
	// The probe runs a real calculation, so this proves the ephemeris files
	// are readable rather than merely that the process is alive.
	if body.Ephemeris.Check != "ok" {
		t.Errorf("ephemeris check = %q, want ok (%s)", body.Ephemeris.Check, body.Ephemeris.Error)
	}
	if body.Ephemeris.Version != "2.10.03" {
		t.Errorf("ephemeris version = %q, want 2.10.03", body.Ephemeris.Version)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}

func TestHealthReportsDraining(t *testing.T) {
	s := newTestServer(t, nil)
	s.draining.Store(true)

	rec := do(t, s, http.MethodGet, "/health", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 while draining", rec.Code)
	}
}

func TestReferenceTopics(t *testing.T) {
	s := newTestServer(t, nil)

	for _, topic := range referenceTopicNames() {
		rec := do(t, s, http.MethodGet, "/v1/reference/"+topic, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", topic, rec.Code)
			continue
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Errorf("%s: could not decode body: %v", topic, err)
		}
		if len(body) == 0 {
			t.Errorf("%s: body is empty", topic)
		}
		if rec.Header().Get("ETag") == "" {
			t.Errorf("%s: no ETag", topic)
		}
	}
}

func TestReferenceUnknownTopic(t *testing.T) {
	s := newTestServer(t, nil)
	rec := do(t, s, http.MethodGet, "/v1/reference/nonsense", nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}

	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("could not decode problem: %v", err)
	}
	if p.Status != http.StatusNotFound {
		t.Errorf("problem status = %d, want 404", p.Status)
	}
	// The error should name the valid options, so a caller can correct the
	// request without reading the documentation.
	if len(p.Errors) == 0 || !strings.Contains(p.Errors[0].Message, "bodies") {
		t.Errorf("problem does not list the valid topics: %+v", p.Errors)
	}
	if p.RequestID == "" {
		t.Error("problem carries no request id")
	}
}

// TestReferenceRevalidates checks that a client holding the current entity tag
// is told it is still current instead of being sent the body again.
func TestReferenceRevalidates(t *testing.T) {
	s := newTestServer(t, nil)

	first := do(t, s, http.MethodGet, "/v1/reference/aspects", nil)
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag on the first response")
	}

	second := do(t, s, http.MethodGet, "/v1/reference/aspects",
		map[string]string{"If-None-Match": etag})
	if second.Code != http.StatusNotModified {
		t.Errorf("status = %d, want 304", second.Code)
	}
	if second.Body.Len() != 0 {
		t.Errorf("304 response carries a body of %d bytes", second.Body.Len())
	}
}

func TestUnknownPathIsProblemJSON(t *testing.T) {
	s := newTestServer(t, nil)
	rec := do(t, s, http.MethodGet, "/v1/does-not-exist", nil)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
}

func TestRequestIDIsEchoed(t *testing.T) {
	s := newTestServer(t, nil)

	rec := do(t, s, http.MethodGet, "/health", nil)
	if rec.Header().Get("X-Request-Id") == "" {
		t.Error("no request id was assigned")
	}

	rec = do(t, s, http.MethodGet, "/health", map[string]string{"X-Request-Id": "abc-123"})
	if got := rec.Header().Get("X-Request-Id"); got != "abc-123" {
		t.Errorf("request id = %q, want the supplied abc-123", got)
	}
}

// TestRequestIDIsSanitized checks that a hostile id is replaced rather than
// written into the logs as given.
func TestRequestIDIsSanitized(t *testing.T) {
	s := newTestServer(t, nil)

	hostile := []string{
		"bad id with spaces",
		"line\nbreak",
		strings.Repeat("x", maxIncomingRequestID+1),
		`{"injected":true}`,
	}
	for _, id := range hostile {
		rec := do(t, s, http.MethodGet, "/health", map[string]string{"X-Request-Id": id})
		got := rec.Header().Get("X-Request-Id")
		if got == id {
			t.Errorf("hostile request id %q was accepted unchanged", id)
		}
		if got == "" {
			t.Errorf("no replacement id was assigned for %q", id)
		}
	}
}

func TestAuthDisabledByDefault(t *testing.T) {
	s := newTestServer(t, nil)
	if rec := do(t, s, http.MethodGet, "/v1", nil); rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 when no keys are configured", rec.Code)
	}
}

func TestAuthRejectsMissingAndWrongKeys(t *testing.T) {
	s := newTestServer(t, func(c *config.Config) {
		c.APIKeys = []string{"secret-key", "other-key"}
	})

	cases := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"no key", nil, http.StatusUnauthorized},
		{"wrong key", map[string]string{"X-API-Key": "nope"}, http.StatusUnauthorized},
		{"empty bearer", map[string]string{"Authorization": "Bearer "}, http.StatusUnauthorized},
		{"valid header key", map[string]string{"X-API-Key": "secret-key"}, http.StatusOK},
		{"valid second key", map[string]string{"X-API-Key": "other-key"}, http.StatusOK},
		{"valid bearer", map[string]string{"Authorization": "Bearer secret-key"}, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, s, http.MethodGet, "/v1", tc.headers)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d; body: %s", rec.Code, tc.want, rec.Body)
			}
		})
	}
}

// TestLicenseIsReachableWithoutKey checks the AGPL section 13 offer stands even
// for a caller with no credentials.
func TestLicenseIsReachableWithoutKey(t *testing.T) {
	s := newTestServer(t, func(c *config.Config) {
		c.APIKeys = []string{"secret-key"}
	})

	rec := do(t, s, http.MethodGet, "/v1/license", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 without a key", rec.Code)
	}

	var body struct {
		License string `json:"license"`
		Source  string `json:"source"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("could not decode body: %v", err)
	}
	if body.License != "AGPL-3.0-or-later" {
		t.Errorf("license = %q", body.License)
	}
	if !strings.HasPrefix(body.Source, "https://github.com/") {
		t.Errorf("source = %q, want a public repository URL", body.Source)
	}
}

func TestHealthIsReachableWithoutKey(t *testing.T) {
	s := newTestServer(t, func(c *config.Config) {
		c.APIKeys = []string{"secret-key"}
	})
	if rec := do(t, s, http.MethodGet, "/health", nil); rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 without a key", rec.Code)
	}
}

func TestCORSDisabledByDefault(t *testing.T) {
	s := newTestServer(t, nil)
	rec := do(t, s, http.MethodGet, "/v1", map[string]string{"Origin": "https://example.com"})

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want none when CORS is off", got)
	}
}

func TestCORSAllowsConfiguredOrigin(t *testing.T) {
	s := newTestServer(t, func(c *config.Config) {
		c.CORSOrigins = []string{"https://astro.example"}
	})

	rec := do(t, s, http.MethodGet, "/v1", map[string]string{"Origin": "https://astro.example"})
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://astro.example" {
		t.Errorf("Access-Control-Allow-Origin = %q", got)
	}
	if !strings.Contains(rec.Header().Get("Vary"), "Origin") {
		t.Error("Vary does not mention Origin, so a cache could serve the wrong origin")
	}

	rec = do(t, s, http.MethodGet, "/v1", map[string]string{"Origin": "https://evil.example"})
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("an unlisted origin was allowed: %q", got)
	}
}

// TestPanicBecomesProblem checks that a fault in a handler returns 500 rather
// than dropping the connection or taking the process down.
func TestPanicBecomesProblem(t *testing.T) {
	s := newTestServer(t, nil)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}), RequestID, Recover(log))

	req := httptest.NewRequest(http.MethodGet, "/v1/natal", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}

	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("could not decode problem: %v", err)
	}
	// The panic value must not reach the caller.
	if strings.Contains(strings.ToLower(p.Detail), "boom") {
		t.Errorf("the response leaks internal detail: %q", p.Detail)
	}
	if p.RequestID == "" {
		t.Error("the response carries no request id to correlate with the log")
	}
	_ = s
}

func TestIndexListsEndpoints(t *testing.T) {
	s := newTestServer(t, nil)
	rec := do(t, s, http.MethodGet, "/v1", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Endpoints []endpoint `json:"endpoints"`
		Source    string     `json:"source"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("could not decode body: %v", err)
	}
	if len(body.Endpoints) == 0 {
		t.Error("the index lists no endpoints")
	}
	if body.Source == "" {
		t.Error("the index does not point at the source")
	}
}

// TestGracefulShutdown checks the shutdown path end to end over a real socket.
//
// It does not use signals: on Windows a SIGTERM sent from a shell is not
// delivered to a native process, so a signal based test would pass by
// accidentally killing the server rather than by draining it.
func TestGracefulShutdown(t *testing.T) {
	s := newTestServer(t, func(c *config.Config) {
		c.Addr = "127.0.0.1:0" // let the system pick a free port
	})

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- s.Run(ctx) }()

	select {
	case <-s.Listening():
	case err := <-runErr:
		t.Fatalf("the server stopped before it listened: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("the server did not start listening")
	}

	base := "http://" + s.Addr().String()
	if resp, err := http.Get(base + "/health"); err != nil {
		t.Fatalf("health request failed: %v", err)
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("health status = %d, want 200", resp.StatusCode)
		}
	}

	cancel()

	select {
	case err := <-runErr:
		if err != nil {
			t.Errorf("Run returned %v, want a clean stop", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after the context was cancelled")
	}

	// The listener must be closed once Run has returned.
	if resp, err := http.Get(base + "/health"); err == nil {
		resp.Body.Close()
		t.Error("the server still accepts connections after shutting down")
	}
}

// TestShutdownWaitsForInFlightRequest checks that a request already running
// when shutdown begins is allowed to finish, rather than being cut off.
func TestShutdownWaitsForInFlightRequest(t *testing.T) {
	s := newTestServer(t, func(c *config.Config) {
		c.Addr = "127.0.0.1:0"
		c.ShutdownTimeout = 5 * time.Second
	})

	// A handler that is still working when shutdown starts.
	released := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		<-released
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("done"))
	})
	s.http.Handler = mux

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- s.Run(ctx) }()
	<-s.Listening()

	base := "http://" + s.Addr().String()
	respCh := make(chan *http.Response, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := http.Get(base + "/slow")
		if err != nil {
			errCh <- err
			return
		}
		respCh <- resp
	}()

	// Give the request time to reach the handler, then begin shutting down
	// while it is still in flight.
	time.Sleep(200 * time.Millisecond)
	cancel()
	time.Sleep(200 * time.Millisecond)
	close(released)

	select {
	case resp := <-respCh:
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK || string(body) != "done" {
			t.Errorf("in-flight request got %d %q, want 200 done", resp.StatusCode, body)
		}
	case err := <-errCh:
		t.Fatalf("the in-flight request was cut off: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the in-flight request never completed")
	}

	if err := <-runErr; err != nil {
		t.Errorf("Run returned %v, want a clean stop", err)
	}
}
