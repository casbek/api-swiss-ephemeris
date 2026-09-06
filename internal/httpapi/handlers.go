package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/casbek/api-swiss-ephemeris/internal/astro"
	"github.com/casbek/api-swiss-ephemeris/internal/libswe"
	"github.com/casbek/api-swiss-ephemeris/internal/swe"
	"github.com/casbek/api-swiss-ephemeris/internal/version"
)

// healthProbeJD is J2000.0, a fixed moment used to prove the ephemeris still
// answers.
const healthProbeJD = 2451545.0

// healthProbeTimeout bounds the readiness check so a stuck worker shows up as
// unhealthy instead of hanging the probe.
const healthProbeTimeout = 2 * time.Second

type healthResponse struct {
	Status        string       `json:"status"`
	UptimeSeconds int64        `json:"uptime_seconds"`
	Version       version.Info `json:"version"`
	Ephemeris     epheHealth   `json:"ephemeris"`
}

type epheHealth struct {
	Version string `json:"version"`
	Check   string `json:"check"`
	Error   string `json:"error,omitempty"`
}

// handleHealth reports whether the service can actually serve requests.
//
// It does not merely answer "the process is up": it runs a real calculation,
// because the failure worth catching is an ephemeris directory that has become
// unreadable while the process kept running.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := healthResponse{
		Status:        "ok",
		UptimeSeconds: int64(time.Since(s.startedAt).Seconds()),
		Version:       version.Get(),
		Ephemeris: epheHealth{
			Version: s.calc.Version(),
			Check:   "ok",
		},
	}

	ctx, cancel := context.WithTimeout(r.Context(), healthProbeTimeout)
	defer cancel()

	err := s.calc.Do(ctx, func(sess *swe.Session) error {
		_, err := sess.Calc(healthProbeJD, libswe.Sun, swe.Options{})
		return err
	})

	status := http.StatusOK
	switch {
	case err != nil:
		resp.Status = "degraded"
		resp.Ephemeris.Check = "failed"
		resp.Ephemeris.Error = err.Error()
		status = http.StatusServiceUnavailable
		s.log.Error("health probe failed", slog.String("error", err.Error()))
	case s.draining.Load():
		// The process is healthy but is being taken out of service.
		resp.Status = "draining"
		status = http.StatusServiceUnavailable
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, r, status, resp)
}

type endpoint struct {
	Path        string `json:"path"`
	Method      string `json:"method"`
	Description string `json:"description"`
}

// handleIndex lists what the service offers, so a client can discover the API
// without reading the documentation first.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		Service   string       `json:"service"`
		Version   version.Info `json:"version"`
		Ephemeris string       `json:"ephemeris"`
		Source    string       `json:"source"`
		License   string       `json:"license"`
		Endpoints []endpoint   `json:"endpoints"`
	}{
		Service:   "api-swiss-ephemeris",
		Version:   version.Get(),
		Ephemeris: "Swiss Ephemeris " + s.calc.Version(),
		Source:    sourceURL,
		License:   licenseID,
		Endpoints: []endpoint{
			{Path: "/health", Method: "GET", Description: "Liveness and readiness"},
			{Path: "/v1", Method: "GET", Description: "This index"},
			{Path: "/v1/license", Method: "GET", Description: "Licence terms and source location"},
			{Path: "/v1/reference/{topic}", Method: "GET", Description: "Supported bodies, signs, house systems, aspects and ayanamshas"},
			{Path: "/v1/time", Method: "POST", Description: "Resolve a local date and time into an instant and a Julian Day"},
			{Path: "/v1/natal", Method: "POST", Description: "Cast a birth chart: bodies, houses, aspects and dignities"},
		},
	}
	writeCacheableJSON(w, r, resp, "public, max-age=300")
}

const (
	sourceURL = "https://github.com/casbek/api-swiss-ephemeris"
	licenseID = "AGPL-3.0-or-later"
)

type component struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	License   string `json:"license"`
	Copyright string `json:"copyright"`
	URL       string `json:"url"`
}

// handleLicence states the licence and where to obtain the source.
//
// Section 13 of the AGPL requires that anyone interacting with the service
// over a network be offered its source. Serving that from the API itself, and
// exempting this path from authentication, means the offer stands even for a
// caller who has no key.
func (s *Server) handleLicense(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		License    string       `json:"license"`
		Source     string       `json:"source"`
		Version    version.Info `json:"version"`
		Notice     string       `json:"notice"`
		Components []component  `json:"components"`
	}{
		License: licenseID,
		Source:  sourceURL,
		Version: version.Get(),
		Notice: "This service is free software under the GNU Affero General Public " +
			"License version 3. Section 13 of that licence entitles anyone " +
			"interacting with it over a network to its complete source code, which " +
			"is published at the address above. If you run a modified version, you " +
			"must offer your modifications to its users in the same way.",
		Components: []component{
			{
				Name:      "Swiss Ephemeris",
				Version:   s.calc.Version(),
				License:   "AGPL-3.0 or Swiss Ephemeris Professional License",
				Copyright: "Copyright (C) 1997-2021 Astrodienst AG, Switzerland. All rights reserved.",
				URL:       "https://www.astro.com/swisseph/",
			},
		},
	}
	writeCacheableJSON(w, r, resp, "public, max-age=3600")
}

// referenceTopics maps a topic name to the data served for it.
var referenceTopics = map[string]func() any{
	"bodies": func() any {
		return map[string]any{
			"bodies":  astro.Bodies(),
			"angles":  astro.Angles(),
			"default": astro.DefaultBodies(),
		}
	},
	"signs": func() any {
		return map[string]any{"signs": astro.Signs()}
	},
	"house-systems": func() any {
		return map[string]any{
			"house_systems": astro.HouseSystems(),
			"default":       astro.DefaultHouseSystem,
		}
	},
	"aspects": func() any {
		return map[string]any{
			"aspects": astro.AspectTypes(),
			"default": astro.DefaultAspects(),
		}
	},
	"ayanamshas": func() any {
		return map[string]any{
			"ayanamshas": astro.Ayanamshas(),
			"default":    astro.DefaultAyanamsha,
		}
	},
}

// referenceTopicNames lists the valid topics, for error messages and for the
// topic index.
func referenceTopicNames() []string {
	names := make([]string, 0, len(referenceTopics))
	for name := range referenceTopics {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// handleReference serves the catalogue, so a client can discover what the
// service supports instead of asking.
func (s *Server) handleReference(w http.ResponseWriter, r *http.Request) {
	topic := r.PathValue("topic")

	build, ok := referenceTopics[topic]
	if !ok {
		p := NotFound("There is no reference topic named " + topic + ".")
		p.Errors = []FieldError{{
			Field:   "topic",
			Message: "must be one of: " + strings.Join(referenceTopicNames(), ", "),
		}}
		WriteProblem(w, r, p)
		return
	}

	// The catalogue only changes when a new build is deployed, so it can be
	// cached hard. The entity tag lets a client revalidate cheaply.
	writeCacheableJSON(w, r, build(), "public, max-age=86400")
}

// handleNotFound answers unknown paths in the same format as every other
// error, rather than the plain text the standard mux would return.
func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	WriteProblem(w, r, NotFound("No endpoint matches "+r.Method+" "+r.URL.Path+"."))
}
