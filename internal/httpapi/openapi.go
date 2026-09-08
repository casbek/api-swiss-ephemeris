package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/casbek/api-swiss-ephemeris/api"
)

// handleOpenAPI serves the OpenAPI description of this build.
//
// It is exempt from authentication for the same reason the licence endpoint
// is: a client deciding whether to integrate needs to read the contract before
// it has a key.
func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	etag := entityTag(api.Spec)
	w.Header().Set("ETag", etag)
	// The document only changes when a new build is deployed, so it can be
	// cached hard and revalidated cheaply.
	w.Header().Set("Cache-Control", "public, max-age=3600")

	if matchesETag(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", api.SpecContentType)
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(api.Spec); err != nil {
		slog.Debug("could not write the OpenAPI document",
			slog.String("request_id", RequestIDFrom(r.Context())),
			slog.String("error", err.Error()))
	}
}
