package httpapi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

// docsBase is the prefix for the URLs used in a problem's type field. Each
// anchor documents one error condition.
const docsBase = "https://github.com/casbek/api-swiss-ephemeris/blob/main/docs/errors.md#"

// Problem is an error response as described by RFC 9457. It is served with the
// application/problem+json content type.
type Problem struct {
	Type     string `json:"type,omitempty"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`

	// Errors carries per field validation failures. It is an extension
	// member, which RFC 9457 allows.
	Errors []FieldError `json:"errors,omitempty"`

	// RequestID lets a caller quote a single request when reporting a
	// problem. Request bodies are never logged, so this is the only handle
	// on a specific call.
	RequestID string `json:"request_id,omitempty"`
}

// FieldError names one invalid input field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Error makes Problem usable as an error, so handlers can return one.
func (p *Problem) Error() string {
	if p.Detail != "" {
		return fmt.Sprintf("%s: %s", p.Title, p.Detail)
	}
	return p.Title
}

// newProblem builds a problem, deriving the type URL from the slug.
func newProblem(status int, slug, title, detail string) *Problem {
	p := &Problem{
		Title:  title,
		Status: status,
		Detail: detail,
	}
	if slug != "" {
		p.Type = docsBase + slug
	}
	return p
}

// BadRequest reports a malformed request, such as a body that is not JSON.
func BadRequest(detail string) *Problem {
	return newProblem(http.StatusBadRequest, "bad-request", "Malformed request", detail)
}

// Unauthorized reports a missing or unrecognised API key.
func Unauthorized(detail string) *Problem {
	return newProblem(http.StatusUnauthorized, "unauthorized", "Unauthorized", detail)
}

// NotFound reports an unknown path.
func NotFound(detail string) *Problem {
	return newProblem(http.StatusNotFound, "not-found", "Not found", detail)
}

// MethodNotAllowed reports a path used with the wrong method.
func MethodNotAllowed(detail string) *Problem {
	return newProblem(http.StatusMethodNotAllowed, "method-not-allowed", "Method not allowed", detail)
}

// Validation reports input that parsed but does not describe a chart that can
// be calculated.
func Validation(detail string, fields ...FieldError) *Problem {
	p := newProblem(http.StatusUnprocessableEntity, "validation-failed", "Validation failed", detail)
	p.Errors = fields
	return p
}

// PayloadTooLarge reports a body over the configured limit.
func PayloadTooLarge(detail string) *Problem {
	return newProblem(http.StatusRequestEntityTooLarge, "payload-too-large", "Request body too large", detail)
}

// Internal reports a fault on the server side. The detail is deliberately
// vague: the specifics belong in the log, not in a response that may reach an
// untrusted caller.
func Internal() *Problem {
	return newProblem(http.StatusInternalServerError, "internal-error", "Internal server error",
		"The request could not be completed. Quote the request id when reporting this.")
}

// ServiceUnavailable reports that the service is running but cannot serve
// requests, for instance while shutting down.
func ServiceUnavailable(detail string) *Problem {
	return newProblem(http.StatusServiceUnavailable, "service-unavailable", "Service unavailable", detail)
}

// WriteProblem sends a problem response, filling in the instance and request
// id from the request.
func WriteProblem(w http.ResponseWriter, r *http.Request, p *Problem) {
	if p == nil {
		p = Internal()
	}
	if p.Instance == "" {
		p.Instance = r.URL.Path
	}
	if p.RequestID == "" {
		p.RequestID = RequestIDFrom(r.Context())
	}

	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	// An error response must never be cached; the next attempt may succeed.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(p.Status)

	if err := json.NewEncoder(w).Encode(p); err != nil {
		// The status line is already sent, so there is nothing to correct.
		// Record it and move on.
		slog.Error("could not write problem response",
			slog.String("request_id", p.RequestID),
			slog.String("error", err.Error()))
	}
}
