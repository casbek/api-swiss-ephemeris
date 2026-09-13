package httpapi

import (
	"net/http"

	"github.com/casbek/api-swiss-ephemeris/internal/astro"
	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// positionsResponse is where the bodies stood at a moment.
type positionsResponse struct {
	Bodies []astro.Position `json:"bodies"`
	Meta   positionsMeta    `json:"meta"`
}

type positionsMeta struct {
	UTC   string  `json:"utc"`
	Local string  `json:"local"`
	Zone  tz.Zone `json:"timezone"`

	JulianDayUT float64 `json:"julian_day_ut"`
	JulianDayTT float64 `json:"julian_day_tt"`

	// Location is reported only when one was given, since these positions do
	// not otherwise depend on a place.
	Location *astro.Location `json:"location,omitempty"`

	Zodiac      string  `json:"zodiac"`
	Ayanamsha   string  `json:"ayanamsha,omitempty"`
	AyanamshaAt float64 `json:"ayanamsha_degrees,omitempty"`
	Topocentric bool    `json:"topocentric,omitempty"`

	Bodies []string `json:"bodies"`

	Obliquity float64 `json:"obliquity"`
	Ephemeris string  `json:"ephemeris"`

	TimeKnown   bool       `json:"time_known"`
	TimeAnomaly tz.Anomaly `json:"time_anomaly,omitempty"`

	Source string `json:"source"`
}

func (s *Server) handlePositions(w http.ResponseWriter, r *http.Request) {
	var req astro.PositionsRequest
	if p := decodeJSON(r, &req); p != nil {
		WriteProblem(w, r, p)
		return
	}

	result, err := s.engine.Positions(r.Context(), req)
	if err != nil {
		WriteProblem(w, r, problemFromFieldError(err))
		return
	}

	resp := positionsResponse{
		Bodies: result.Positions,
		Meta: positionsMeta{
			UTC:         result.Moment.UTC.Format(rfc3339),
			Local:       result.Moment.Local.Format(rfc3339),
			Zone:        result.Moment.Zone,
			JulianDayUT: result.Moment.JulianDayUT,
			JulianDayTT: result.Moment.JulianDayTT,
			Zodiac:      string(result.Settings.Zodiac),
			Topocentric: result.Settings.Topocentric,
			Bodies:      result.Settings.BodyNames(),
			Obliquity:   result.Obliquity,
			Ephemeris:   "Swiss Ephemeris " + s.engine.EphemerisVersion(),
			TimeKnown:   result.Moment.TimeKnown,
			TimeAnomaly: result.Moment.Anomaly,
			Source:      sourceURL,
		},
	}
	if req.Location != nil {
		resp.Meta.Location = &result.Location
	}
	if result.Settings.Zodiac == astro.ZodiacSidereal {
		resp.Meta.Ayanamsha = result.Settings.Ayanamsha.Name
		resp.Meta.AyanamshaAt = result.Ayanamsha
	}

	writeJSON(w, r, http.StatusOK, resp)
}

// housesResponse is the division of the sky at a place and a moment.
type housesResponse struct {
	Houses *astro.Houses `json:"houses"`
	Meta   housesMeta    `json:"meta"`
}

type housesMeta struct {
	UTC   string  `json:"utc"`
	Local string  `json:"local"`
	Zone  tz.Zone `json:"timezone"`

	JulianDayUT float64 `json:"julian_day_ut"`
	JulianDayTT float64 `json:"julian_day_tt"`

	Location astro.Location `json:"location"`

	Zodiac      string  `json:"zodiac"`
	Ayanamsha   string  `json:"ayanamsha,omitempty"`
	AyanamshaAt float64 `json:"ayanamsha_degrees,omitempty"`
	HouseSystem string  `json:"house_system"`

	Obliquity float64 `json:"obliquity"`
	Ephemeris string  `json:"ephemeris"`

	TimeAnomaly tz.Anomaly `json:"time_anomaly,omitempty"`

	Source string `json:"source"`
}

func (s *Server) handleHouses(w http.ResponseWriter, r *http.Request) {
	var req astro.HousesRequest
	if p := decodeJSON(r, &req); p != nil {
		WriteProblem(w, r, p)
		return
	}

	result, err := s.engine.Houses(r.Context(), req)
	if err != nil {
		WriteProblem(w, r, problemFromFieldError(err))
		return
	}

	resp := housesResponse{
		Houses: result.Houses,
		Meta: housesMeta{
			UTC:         result.Moment.UTC.Format(rfc3339),
			Local:       result.Moment.Local.Format(rfc3339),
			Zone:        result.Moment.Zone,
			JulianDayUT: result.Moment.JulianDayUT,
			JulianDayTT: result.Moment.JulianDayTT,
			Location:    result.Location,
			Zodiac:      string(result.Settings.Zodiac),
			HouseSystem: result.Settings.HouseSystem.Name,
			Obliquity:   result.Obliquity,
			Ephemeris:   "Swiss Ephemeris " + s.engine.EphemerisVersion(),
			TimeAnomaly: result.Moment.Anomaly,
			Source:      sourceURL,
		},
	}
	if result.Settings.Zodiac == astro.ZodiacSidereal {
		resp.Meta.Ayanamsha = result.Settings.Ayanamsha.Name
		resp.Meta.AyanamshaAt = result.Ayanamsha
	}

	writeJSON(w, r, http.StatusOK, resp)
}
