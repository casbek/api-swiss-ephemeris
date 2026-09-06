package httpapi

import (
	"net/http"

	"github.com/casbek/api-swiss-ephemeris/internal/astro"
)

// transitResponse is a birth chart, the sky at another moment, and how they
// relate.
type transitResponse struct {
	Natal   natalResponse `json:"natal"`
	Transit natalResponse `json:"transit"`

	// Aspects run from a transiting body to a natal point: from is always the
	// transiting side and to always the natal one.
	Aspects []astro.Aspect `json:"aspects"`
}

func (s *Server) handleTransits(w http.ResponseWriter, r *http.Request) {
	var req astro.TransitRequest
	if p := decodeJSON(r, &req); p != nil {
		WriteProblem(w, r, p)
		return
	}

	result, err := s.engine.Transits(r.Context(), req)
	if err != nil {
		WriteProblem(w, r, problemFromFieldError(err))
		return
	}

	version := s.engine.EphemerisVersion()
	writeJSON(w, r, http.StatusOK, transitResponse{
		Natal:   buildNatalResponse(result.Natal, version),
		Transit: buildNatalResponse(result.Transit, version),
		Aspects: orEmpty(result.Aspects),
	})
}

// synastryResponse is two charts and the contacts between them.
type synastryResponse struct {
	ChartA natalResponse `json:"chart_a"`
	ChartB natalResponse `json:"chart_b"`

	// Aspects run from a point in the first chart to a point in the second.
	Aspects []astro.Aspect `json:"aspects"`
}

func (s *Server) handleSynastry(w http.ResponseWriter, r *http.Request) {
	var req astro.SynastryRequest
	if p := decodeJSON(r, &req); p != nil {
		WriteProblem(w, r, p)
		return
	}

	result, err := s.engine.Synastry(r.Context(), req)
	if err != nil {
		WriteProblem(w, r, problemFromFieldError(err))
		return
	}

	version := s.engine.EphemerisVersion()
	writeJSON(w, r, http.StatusOK, synastryResponse{
		ChartA:  buildNatalResponse(result.ChartA, version),
		ChartB:  buildNatalResponse(result.ChartB, version),
		Aspects: orEmpty(result.Aspects),
	})
}

// compositeResponse is a relationship chart.
type compositeResponse struct {
	Method string `json:"method"`

	Bodies       []astro.Position   `json:"bodies"`
	Houses       *astro.Houses      `json:"houses,omitempty"`
	Aspects      []astro.Aspect     `json:"aspects"`
	Distribution astro.Distribution `json:"distribution"`
	Sect         astro.Sect         `json:"sect,omitempty"`
	Notes        []string           `json:"notes,omitempty"`

	ChartA natalResponse `json:"chart_a"`
	ChartB natalResponse `json:"chart_b"`

	Meta compositeMeta `json:"meta"`
}

type compositeMeta struct {
	Method      string   `json:"method"`
	Zodiac      string   `json:"zodiac"`
	Ayanamsha   string   `json:"ayanamsha,omitempty"`
	HouseSystem string   `json:"house_system,omitempty"`
	Bodies      []string `json:"bodies"`
	AspectTypes []string `json:"aspect_types"`
	Ephemeris   string   `json:"ephemeris"`
	Source      string   `json:"source"`

	// A Davison chart is cast for a real instant at a real place, so it has
	// a moment and a location to report. A midpoint composite has neither.
	UTC       string          `json:"utc,omitempty"`
	Location  *astro.Location `json:"location,omitempty"`
	JulianDay float64         `json:"julian_day_ut,omitempty"`
}

func (s *Server) handleComposite(w http.ResponseWriter, r *http.Request) {
	var req astro.CompositeRequest
	if p := decodeJSON(r, &req); p != nil {
		WriteProblem(w, r, p)
		return
	}

	result, err := s.engine.Composite(r.Context(), req)
	if err != nil {
		WriteProblem(w, r, problemFromFieldError(err))
		return
	}

	version := s.engine.EphemerisVersion()
	resp := compositeResponse{
		Method:       string(result.Method),
		Bodies:       result.Positions,
		Houses:       result.Houses,
		Aspects:      orEmpty(result.Aspects),
		Distribution: result.Distribution,
		Sect:         result.Sect,
		Notes:        result.Notes,
		ChartA:       buildNatalResponse(result.ChartA, version),
		ChartB:       buildNatalResponse(result.ChartB, version),
		Meta: compositeMeta{
			Method:      string(result.Method),
			Zodiac:      string(result.Settings.Zodiac),
			Bodies:      result.Settings.BodyNames(),
			AspectTypes: result.Settings.AspectNames(),
			Ephemeris:   "Swiss Ephemeris " + version,
			Source:      sourceURL,
		},
	}
	if result.Houses != nil {
		resp.Meta.HouseSystem = result.Settings.HouseSystem.Name
	}
	if result.Settings.Zodiac == astro.ZodiacSidereal {
		resp.Meta.Ayanamsha = result.Settings.Ayanamsha.Name
	}
	if result.Moment != nil {
		resp.Meta.UTC = result.Moment.UTC.Format(rfc3339)
		resp.Meta.JulianDay = result.Moment.JulianDayUT
	}
	resp.Meta.Location = result.Location

	writeJSON(w, r, http.StatusOK, resp)
}

// orEmpty makes a nil aspect slice serialise as [] rather than null, so a
// client can iterate without a guard.
func orEmpty(a []astro.Aspect) []astro.Aspect {
	if a == nil {
		return []astro.Aspect{}
	}
	return a
}
