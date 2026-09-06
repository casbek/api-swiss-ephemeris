package httpapi

import (
	"net/http"

	"github.com/casbek/api-swiss-ephemeris/internal/astro"
)

// progressionResponse is a birth chart moved forward to a later date.
type progressionResponse struct {
	Method string `json:"method"`

	Bodies  []astro.Position `json:"bodies"`
	Houses  *astro.Houses    `json:"houses,omitempty"`
	Aspects []astro.Aspect   `json:"aspects"`
	Notes   []string         `json:"notes,omitempty"`

	Natal natalResponse   `json:"natal"`
	Meta  progressionMeta `json:"meta"`
}

type progressionMeta struct {
	Method string `json:"method"`

	// TargetUTC is the date progressed to and ElapsedYears the age at it.
	TargetUTC    string  `json:"target_utc"`
	ElapsedYears float64 `json:"elapsed_years"`

	// ProgressedUTC is the moment the secondary chart was cast for, a few
	// weeks after the birth. A solar arc is not cast for a moment, and
	// reports the arc it moved by instead.
	ProgressedUTC string  `json:"progressed_utc,omitempty"`
	ArcDegrees    float64 `json:"arc_degrees,omitempty"`

	Zodiac      string   `json:"zodiac"`
	Ayanamsha   string   `json:"ayanamsha,omitempty"`
	HouseSystem string   `json:"house_system,omitempty"`
	Bodies      []string `json:"bodies"`
	AspectTypes []string `json:"aspect_types"`
	Ephemeris   string   `json:"ephemeris"`
	Source      string   `json:"source"`
}

func (s *Server) handleProgressions(w http.ResponseWriter, r *http.Request) {
	var req astro.ProgressionRequest
	if p := decodeJSON(r, &req); p != nil {
		WriteProblem(w, r, p)
		return
	}

	result, err := s.engine.Progress(r.Context(), req)
	if err != nil {
		WriteProblem(w, r, problemFromFieldError(err))
		return
	}

	version := s.engine.EphemerisVersion()
	resp := progressionResponse{
		Method:  string(result.Method),
		Bodies:  result.Positions,
		Houses:  result.Houses,
		Aspects: orEmpty(result.Aspects),
		Notes:   result.Notes,
		Natal:   buildNatalResponse(result.Natal, version),
		Meta: progressionMeta{
			Method:       string(result.Method),
			TargetUTC:    result.Target.UTC.Format(rfc3339),
			ElapsedYears: result.ElapsedYears,
			Zodiac:       string(result.Settings.Zodiac),
			Bodies:       result.Settings.BodyNames(),
			AspectTypes:  result.Settings.AspectNames(),
			Ephemeris:    "Swiss Ephemeris " + version,
			Source:       sourceURL,
		},
	}
	if result.ProgressedMoment != nil {
		resp.Meta.ProgressedUTC = result.ProgressedMoment.UTC.Format(rfc3339)
	}
	if result.Method == astro.ProgressionSolarArc {
		resp.Meta.ArcDegrees = result.Arc
	}
	if result.Houses != nil {
		resp.Meta.HouseSystem = result.Settings.HouseSystem.Name
	}
	if result.Settings.Zodiac == astro.ZodiacSidereal {
		resp.Meta.Ayanamsha = result.Settings.Ayanamsha.Name
	}

	writeJSON(w, r, http.StatusOK, resp)
}

// returnsResponse is a set of return charts.
type returnsResponse struct {
	Body    string        `json:"body"`
	Natal   natalResponse `json:"natal"`
	Returns []returnEntry `json:"returns"`
	Meta    returnsMeta   `json:"meta"`
}

type returnEntry struct {
	// ExactUTC is the moment the body reached its natal degree.
	ExactUTC string `json:"exact_utc"`

	// NatalLongitude is the degree it returned to.
	NatalLongitude float64 `json:"natal_longitude"`

	Chart natalResponse `json:"chart"`
}

type returnsMeta struct {
	Body         string         `json:"body"`
	Count        int            `json:"count"`
	SearchedFrom string         `json:"searched_from"`
	Location     astro.Location `json:"location"`
	Ephemeris    string         `json:"ephemeris"`
	Source       string         `json:"source"`
}

func (s *Server) handleReturns(w http.ResponseWriter, r *http.Request) {
	var req astro.ReturnRequest
	if p := decodeJSON(r, &req); p != nil {
		WriteProblem(w, r, p)
		return
	}

	result, err := s.engine.Returns(r.Context(), req)
	if err != nil {
		WriteProblem(w, r, problemFromFieldError(err))
		return
	}

	version := s.engine.EphemerisVersion()
	resp := returnsResponse{
		Body:    result.Body,
		Natal:   buildNatalResponse(result.Natal, version),
		Returns: make([]returnEntry, 0, len(result.Returns)),
		Meta: returnsMeta{
			Body:         result.Body,
			Count:        len(result.Returns),
			SearchedFrom: result.SearchedFrom.UTC.Format(rfc3339),
			Ephemeris:    "Swiss Ephemeris " + version,
			Source:       sourceURL,
		},
	}
	for _, rc := range result.Returns {
		resp.Returns = append(resp.Returns, returnEntry{
			ExactUTC:       rc.Chart.Moment.UTC.Format(rfc3339),
			NatalLongitude: rc.NatalLongitude,
			Chart:          buildNatalResponse(rc.Chart, version),
		})
	}
	if len(result.Returns) > 0 {
		resp.Meta.Location = result.Returns[0].Chart.Location
	}

	writeJSON(w, r, http.StatusOK, resp)
}
