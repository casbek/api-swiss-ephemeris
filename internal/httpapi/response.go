package httpapi

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
)

// writeJSON sends v as JSON with the given status.
func writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		slog.Error("could not encode response",
			slog.String("request_id", RequestIDFrom(r.Context())),
			slog.String("path", r.URL.Path),
			slog.String("error", err.Error()))
		WriteProblem(w, r, Internal())
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(body); err != nil {
		// The client went away mid response. Nothing to fix, and it is not
		// a server fault, so record it quietly.
		slog.Debug("could not write response",
			slog.String("request_id", RequestIDFrom(r.Context())),
			slog.String("error", err.Error()))
	}
}

// writeCacheableJSON sends v as JSON with an entity tag and a cache lifetime,
// and answers a matching If-None-Match with 304.
//
// Results here are deterministic, so a client that has seen a value never
// needs to fetch it again.
func writeCacheableJSON(w http.ResponseWriter, r *http.Request, v any, cacheControl string) {
	body, err := json.Marshal(v)
	if err != nil {
		slog.Error("could not encode response",
			slog.String("request_id", RequestIDFrom(r.Context())),
			slog.String("path", r.URL.Path),
			slog.String("error", err.Error()))
		WriteProblem(w, r, Internal())
		return
	}

	etag := entityTag(body)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", cacheControl)

	if matchesETag(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(body); err != nil {
		slog.Debug("could not write response",
			slog.String("request_id", RequestIDFrom(r.Context())),
			slog.String("error", err.Error()))
	}
}

// entityTag derives a strong entity tag from a response body.
func entityTag(body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + base64.RawURLEncoding.EncodeToString(sum[:16]) + `"`
}

// matchesETag reports whether an If-None-Match header covers the given tag.
func matchesETag(header, etag string) bool {
	if header == "" {
		return false
	}
	if strings.TrimSpace(header) == "*" {
		return true
	}
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		// A weak validator still identifies the same representation for
		// this purpose.
		candidate = strings.TrimPrefix(candidate, "W/")
		if candidate == etag {
			return true
		}
	}
	return false
}

// decodeJSON reads a JSON request body into v, rejecting unknown fields.
//
// Unknown fields are an error on purpose: a caller who misspells a setting
// should be told, not silently given default behaviour and a chart that is
// subtly not what they asked for.
func decodeJSON(r *http.Request, v any) *Problem {
	if ct := r.Header.Get("Content-Type"); ct != "" {
		if mediaType, _, _ := strings.Cut(ct, ";"); strings.TrimSpace(mediaType) != "application/json" {
			return BadRequest("The request body must be sent as application/json.")
		}
	}

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(v); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return PayloadTooLarge("The request body exceeds the size this service accepts.")
		}
		return BadRequest("The request body is not valid JSON: " + err.Error())
	}
	// A second value in the stream means the caller sent something other
	// than one JSON object.
	if dec.More() {
		return BadRequest("The request body must contain exactly one JSON object.")
	}
	return nil
}
