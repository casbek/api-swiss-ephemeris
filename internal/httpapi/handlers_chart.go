package httpapi

import (
	"net/http"
	"time"

	"github.com/casbek/api-swiss-ephemeris/internal/astro"
	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// natalResponse is a cast chart.
type natalResponse struct {
	Bodies       []astro.Position   `json:"bodies"`
	Houses       *astro.Houses      `json:"houses,omitempty"`
	Aspects      []astro.Aspect     `json:"aspects"`
	Distribution astro.Distribution `json:"distribution"`
	Sect         astro.Sect         `json:"sect,omitempty"`
	Notes        []string           `json:"notes,omitempty"`
	Meta         chartMeta          `json:"meta"`
}

// chartMeta records how the chart was produced.
//
// It carries every input that could change a result: the instant, the zone the
// instant came from, the zodiac, the house system and the bodies asked for.
// When a client says the chart disagrees with another service, the difference
// is almost always one of these, and having them in the response settles it
// without a conversation.
type chartMeta struct {
	UTC   string  `json:"utc"`
	Local string  `json:"local"`
	Zone  tz.Zone `json:"timezone"`

	JulianDayUT float64 `json:"julian_day_ut"`
	JulianDayTT float64 `json:"julian_day_tt"`

	Location astro.Location `json:"location"`

	Zodiac      string  `json:"zodiac"`
	Ayanamsha   string  `json:"ayanamsha,omitempty"`
	AyanamshaAt float64 `json:"ayanamsha_degrees,omitempty"`
	HouseSystem string  `json:"house_system,omitempty"`

	Bodies      []string `json:"bodies"`
	AspectTypes []string `json:"aspect_types"`

	Obliquity float64 `json:"obliquity"`
	Ephemeris string  `json:"ephemeris"`

	// TimeKnown is false when no birth time was given. TimeAnomaly is set
	// when the clocks skipped or repeated the reading.
	TimeKnown   bool       `json:"time_known"`
	TimeAnomaly tz.Anomaly `json:"time_anomaly,omitempty"`

	Source string `json:"source"`
}

// handleNatal casts a birth chart.
func (s *Server) handleNatal(w http.ResponseWriter, r *http.Request) {
	var req astro.ChartRequest
	if p := decodeJSON(r, &req); p != nil {
		WriteProblem(w, r, p)
		return
	}

	chart, err := s.engine.Cast(r.Context(), req)
	if err != nil {
		WriteProblem(w, r, problemFromFieldError(err))
		return
	}

	writeJSON(w, r, http.StatusOK, buildNatalResponse(chart, s.engine.EphemerisVersion()))
}

// buildNatalResponse turns a chart into its response shape.
func buildNatalResponse(c *astro.Chart, ephemeris string) natalResponse {
	resp := natalResponse{
		Bodies:       c.Positions,
		Houses:       c.Houses,
		Aspects:      c.Aspects,
		Distribution: c.Distribution,
		Sect:         c.Sect,
		Notes:        notesFor(c.Moment),
		Meta: chartMeta{
			UTC:         c.Moment.UTC.Format(time.RFC3339),
			Local:       c.Moment.Local.Format(time.RFC3339),
			Zone:        c.Moment.Zone,
			JulianDayUT: c.Moment.JulianDayUT,
			JulianDayTT: c.Moment.JulianDayTT,
			Location:    c.Location,
			Zodiac:      string(c.Settings.Zodiac),
			Bodies:      c.Settings.BodyNames(),
			AspectTypes: c.Settings.AspectNames(),
			Obliquity:   c.Obliquity,
			Ephemeris:   "Swiss Ephemeris " + ephemeris,
			TimeKnown:   c.Moment.TimeKnown,
			TimeAnomaly: c.Moment.Anomaly,
			Source:      sourceURL,
		},
	}

	// The house system is only meaningful when houses were calculated, and
	// the ayanamsha only in a sidereal chart. Reporting either otherwise
	// would suggest it affected a result it did not.
	if c.Houses != nil {
		resp.Meta.HouseSystem = c.Settings.HouseSystem.Name
	}
	if c.Settings.Zodiac == astro.ZodiacSidereal {
		resp.Meta.Ayanamsha = c.Settings.Ayanamsha.Name
		resp.Meta.AyanamshaAt = c.Ayanamsha
	}

	// An empty aspect list should serialise as [] rather than null, so a
	// client can iterate without a nil check.
	if resp.Aspects == nil {
		resp.Aspects = []astro.Aspect{}
	}
	return resp
}
