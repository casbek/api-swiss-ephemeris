package httpapi

import (
	"net/http"

	"github.com/casbek/api-swiss-ephemeris/internal/astro"
)

// transitSearchResponse lists when the transiting bodies contact a chart.
type transitSearchResponse struct {
	Contacts []transitContact `json:"contacts"`

	Natal natalResponse     `json:"natal"`
	Meta  transitSearchMeta `json:"meta"`
}

type transitContact struct {
	// Transiting names the moving body and Natal the point it reaches.
	Transiting string `json:"transiting"`
	Natal      string `json:"natal"`

	Aspect string  `json:"aspect"`
	Angle  float64 `json:"angle"`

	ExactUTC    string  `json:"exact_utc"`
	JulianDayUT float64 `json:"julian_day_ut"`

	TransitingLongitude float64 `json:"transiting_longitude"`
	NatalLongitude      float64 `json:"natal_longitude"`

	// Retrograde is whether the transiting body was moving backwards, which
	// is what tells the middle pass of a triple contact from the two either
	// side of it.
	Retrograde bool `json:"retrograde"`

	// Pass numbers the contacts of this body, point and aspect within the span
	// searched, and Passes counts them.
	Pass   int `json:"pass"`
	Passes int `json:"passes"`
}

type transitSearchMeta struct {
	From string `json:"from"`
	To   string `json:"to"`

	Bodies      []string `json:"bodies"`
	Points      []string `json:"points"`
	AspectTypes []string `json:"aspect_types"`

	Count int `json:"count"`

	// Truncated is set when the search stopped at the limit rather than at the
	// end of the span. Narrow the bodies, the points or the aspects to see the
	// rest.
	Truncated bool `json:"truncated,omitempty"`

	Zodiac      string `json:"zodiac"`
	Ayanamsha   string `json:"ayanamsha,omitempty"`
	HouseSystem string `json:"house_system,omitempty"`

	Ephemeris string `json:"ephemeris"`
	Source    string `json:"source"`
}

func (s *Server) handleTransitSearch(w http.ResponseWriter, r *http.Request) {
	var req astro.TransitSearchRequest
	if p := decodeJSON(r, &req); p != nil {
		WriteProblem(w, r, p)
		return
	}

	result, err := s.engine.SearchTransits(r.Context(), req)
	if err != nil {
		WriteProblem(w, r, problemFromFieldError(err))
		return
	}

	version := s.engine.EphemerisVersion()
	resp := transitSearchResponse{
		Contacts: make([]transitContact, 0, len(result.Contacts)),
		Natal:    buildNatalResponse(result.Natal, version),
		Meta: transitSearchMeta{
			From:        result.From.UTC.Format(rfc3339),
			To:          result.To.UTC.Format(rfc3339),
			Bodies:      result.Bodies,
			Points:      result.Points,
			AspectTypes: result.Settings.AspectNames(),
			Count:       len(result.Contacts),
			Truncated:   result.Truncated,
			Zodiac:      string(result.Settings.Zodiac),
			Ephemeris:   "Swiss Ephemeris " + version,
			Source:      sourceURL,
		},
	}
	if result.Natal.Houses != nil {
		resp.Meta.HouseSystem = result.Settings.HouseSystem.Name
	}
	if result.Settings.Zodiac == astro.ZodiacSidereal {
		resp.Meta.Ayanamsha = result.Settings.Ayanamsha.Name
	}

	for _, c := range result.Contacts {
		resp.Contacts = append(resp.Contacts, transitContact{
			Transiting:          c.Transiting,
			Natal:               c.Natal,
			Aspect:              c.Aspect,
			Angle:               c.Angle,
			ExactUTC:            c.ExactAt.Format(rfc3339),
			JulianDayUT:         c.JulianDayUT,
			TransitingLongitude: c.TransitingLongitude,
			NatalLongitude:      c.NatalLongitude,
			Retrograde:          c.Retrograde,
			Pass:                c.Pass,
			Passes:              c.Passes,
		})
	}

	writeJSON(w, r, http.StatusOK, resp)
}
