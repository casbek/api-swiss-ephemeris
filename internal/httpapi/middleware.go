package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

type contextKey string

const requestIDKey contextKey = "request_id"

// maxIncomingRequestID caps the length of a client supplied request id.
const maxIncomingRequestID = 64

// RequestIDFrom returns the request id carried in a context, or "" if there is
// none.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// newRequestID returns a random hex identifier.
func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail in practice, and an id is only a
		// correlation handle, so fall back to the clock rather than
		// failing the request.
		return hex.EncodeToString([]byte(time.Now().Format("150405.000000")))
	}
	return hex.EncodeToString(b[:])
}

// sanitizeRequestID keeps a client supplied id only if it is short and made of
// safe characters.
//
// The id is written into structured logs, so accepting arbitrary bytes would
// let a caller forge log entries or smuggle control characters into whatever
// reads them.
func sanitizeRequestID(s string) string {
	if s == "" || len(s) > maxIncomingRequestID {
		return ""
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c >= 'a' && c <= 'z' ||
			c >= 'A' && c <= 'Z' ||
			c >= '0' && c <= '9' ||
			c == '-' || c == '_'
		if !ok {
			return ""
		}
	}
	return s
}

// RequestID assigns an identifier to every request and echoes it back, so a
// caller can quote one specific call when reporting a problem.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := sanitizeRequestID(r.Header.Get("X-Request-Id"))
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set("X-Request-Id", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// statusRecorder captures what a handler wrote, for the access log.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written int64
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
		s.ResponseWriter.WriteHeader(code)
	}
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.written += int64(n)
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Recover turns a panic in a handler into a 500 response instead of dropping
// the connection and taking the process down.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				// ErrAbortHandler is how a handler asks to drop the
				// connection on purpose; it is not a fault.
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				log.Error("handler panicked",
					slog.String("request_id", RequestIDFrom(r.Context())),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.Any("panic", rec),
					slog.String("stack", string(debug.Stack())))
				WriteProblem(w, r, Internal())
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// AccessLog records one line per request.
//
// Only the method, path, status, duration and request id are recorded. Request
// bodies are never logged: they carry birth dates, times and coordinates,
// which together identify a person, and a log file is the wrong place for
// that. Debugging is done through the request id instead.
func AccessLog(log *slog.Logger, skipPaths ...string) func(http.Handler) http.Handler {
	skip := make(map[string]bool, len(skipPaths))
	for _, p := range skipPaths {
		skip[p] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if skip[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)

			if rec.status == 0 {
				rec.status = http.StatusOK
			}
			level := slog.LevelInfo
			switch {
			case rec.status >= 500:
				level = slog.LevelError
			case rec.status >= 400:
				level = slog.LevelWarn
			}

			log.LogAttrs(r.Context(), level, "request",
				slog.String("request_id", RequestIDFrom(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Int64("bytes", rec.written),
				slog.Duration("duration", time.Since(start)))
		})
	}
}

// MaxBody rejects a request body over the limit before a handler reads it.
func MaxBody(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > limit {
				WriteProblem(w, r, PayloadTooLarge(
					"The request body exceeds the size this service accepts."))
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

// APIKeyAuth requires a recognised key on every request except the paths in
// exempt.
//
// The key may be sent as X-API-Key or as an Authorization bearer token.
// Comparison is constant time so a wrong key cannot be recovered by timing the
// response.
func APIKeyAuth(keys []string, exempt ...string) func(http.Handler) http.Handler {
	free := make(map[string]bool, len(exempt))
	for _, p := range exempt {
		free[p] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if free[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			presented := r.Header.Get("X-API-Key")
			if presented == "" {
				if after, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
					presented = strings.TrimSpace(after)
				}
			}
			if presented == "" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="api"`)
				WriteProblem(w, r, Unauthorized(
					"Send an API key in the X-API-Key header or as a bearer token."))
				return
			}

			// Every candidate is compared, so the time taken does not
			// depend on which key matched or how far the scan went.
			var matched bool
			for _, k := range keys {
				if subtle.ConstantTimeCompare([]byte(k), []byte(presented)) == 1 {
					matched = true
				}
			}
			if !matched {
				WriteProblem(w, r, Unauthorized("The API key is not recognised."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORS allows browser requests from the configured origins. An empty list
// disables it, which is the right setting when only server side clients call
// the service.
func CORS(origins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(origins))
	wildcard := false
	for _, o := range origins {
		if o == "*" {
			wildcard = true
		}
		allowed[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(allowed) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			origin := r.Header.Get("Origin")
			if origin != "" && (wildcard || allowed[origin]) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, Authorization, X-Request-Id")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Max-Age", "86400")
				// The response depends on the request's Origin, so caches
				// must key on it.
				w.Header().Add("Vary", "Origin")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// chain applies middleware so that the first listed runs outermost.
func chain(h http.Handler, mw ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}
