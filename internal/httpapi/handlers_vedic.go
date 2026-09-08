package httpapi

import (
	"net/http"

	"github.com/casbek/api-swiss-ephemeris/internal/astro"
)

// dashaResponse is the Vimshottari sequence of a birth.
type dashaResponse struct {
	// Periods are the great periods, each holding its divisions to the depth
	// asked for.
	Periods []astro.DashaPeriod `json:"periods"`

	Natal natalResponse `json:"natal"`
	Meta  dashaMeta     `json:"meta"`
}

type dashaMeta struct {
	// Moon and Nakshatra are what the whole sequence was read from: which
	// mansion the Moon stood in at birth, and how far through it.
	Moon      astro.Position           `json:"moon"`
	Nakshatra astro.NakshatraPlacement `json:"nakshatra"`

	// StartingLord opens the sequence, and BalanceYears is how much of its
	// period remained unspent at birth.
	StartingLord string  `json:"starting_lord"`
	BalanceYears float64 `json:"balance_years"`

	// YearLength is how a dasha year was measured. It is reported because it
	// is the largest single reason two pieces of software put the period
	// boundaries in different places.
	YearLength     string  `json:"year_length"`
	YearLengthDays float64 `json:"year_length_days"`

	Depth     int    `json:"depth"`
	Zodiac    string `json:"zodiac"`
	Ayanamsha string `json:"ayanamsha"`
	Ephemeris string `json:"ephemeris"`
	Source    string `json:"source"`
}

func (s *Server) handleDashas(w http.ResponseWriter, r *http.Request) {
	var req astro.DashaRequest
	if p := decodeJSON(r, &req); p != nil {
		WriteProblem(w, r, p)
		return
	}

	result, err := s.engine.Dashas(r.Context(), req)
	if err != nil {
		WriteProblem(w, r, problemFromFieldError(err))
		return
	}

	version := s.engine.EphemerisVersion()
	writeJSON(w, r, http.StatusOK, dashaResponse{
		Periods: result.Periods,
		Natal:   buildNatalResponse(result.Natal, version),
		Meta: dashaMeta{
			Moon:           result.Moon,
			Nakshatra:      result.Nakshatra,
			StartingLord:   result.StartingLord,
			BalanceYears:   result.BalanceYears,
			YearLength:     string(result.YearLength),
			YearLengthDays: result.YearLengthDays,
			Depth:          result.Depth,
			Zodiac:         string(result.Natal.Settings.Zodiac),
			Ayanamsha:      result.Natal.Settings.Ayanamsha.Name,
			Ephemeris:      "Swiss Ephemeris " + version,
			Source:         sourceURL,
		},
	})
}

// divisionalResponse is a set of divisional charts.
type divisionalResponse struct {
	Charts []astro.VargaChart `json:"charts"`

	Natal natalResponse  `json:"natal"`
	Meta  divisionalMeta `json:"meta"`
}

type divisionalMeta struct {
	Charts    []string `json:"charts"`
	Zodiac    string   `json:"zodiac"`
	Ayanamsha string   `json:"ayanamsha"`
	Ephemeris string   `json:"ephemeris"`
	Source    string   `json:"source"`
}

func (s *Server) handleDivisionals(w http.ResponseWriter, r *http.Request) {
	var req astro.VargaRequest
	if p := decodeJSON(r, &req); p != nil {
		WriteProblem(w, r, p)
		return
	}

	result, err := s.engine.Divisionals(r.Context(), req)
	if err != nil {
		WriteProblem(w, r, problemFromFieldError(err))
		return
	}

	version := s.engine.EphemerisVersion()
	names := make([]string, 0, len(result.Charts))
	for _, c := range result.Charts {
		names = append(names, c.Name)
	}

	writeJSON(w, r, http.StatusOK, divisionalResponse{
		Charts: result.Charts,
		Natal:  buildNatalResponse(result.Natal, version),
		Meta: divisionalMeta{
			Charts:    names,
			Zodiac:    string(result.Natal.Settings.Zodiac),
			Ayanamsha: result.Natal.Settings.Ayanamsha.Name,
			Ephemeris: "Swiss Ephemeris " + version,
			Source:    sourceURL,
		},
	})
}
