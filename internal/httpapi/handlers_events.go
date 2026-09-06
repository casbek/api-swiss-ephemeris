package httpapi

import (
	"net/http"
	"time"

	"github.com/casbek/api-swiss-ephemeris/internal/astro"
)

// ephemerisResponse is a table of positions at regular intervals.
type ephemerisResponse struct {
	Rows []ephemerisRow `json:"rows"`
	Meta ephemerisMeta  `json:"meta"`
}

type ephemerisRow struct {
	UTC         string           `json:"utc"`
	JulianDayUT float64          `json:"julian_day_ut"`
	Bodies      []astro.Position `json:"bodies"`
}

type ephemerisMeta struct {
	From      string   `json:"from"`
	To        string   `json:"to"`
	StepDays  float64  `json:"step_days"`
	Rows      int      `json:"rows"`
	Zodiac    string   `json:"zodiac"`
	Ayanamsha string   `json:"ayanamsha,omitempty"`
	Bodies    []string `json:"bodies"`
	Ephemeris string   `json:"ephemeris"`
	Source    string   `json:"source"`
}

func (s *Server) handleEphemeris(w http.ResponseWriter, r *http.Request) {
	var req astro.EphemerisRequest
	if p := decodeJSON(r, &req); p != nil {
		WriteProblem(w, r, p)
		return
	}

	result, err := s.engine.Ephemeris(r.Context(), req)
	if err != nil {
		WriteProblem(w, r, problemFromFieldError(err))
		return
	}

	resp := ephemerisResponse{
		Rows: make([]ephemerisRow, 0, len(result.Rows)),
		Meta: ephemerisMeta{
			StepDays:  result.StepDays,
			Rows:      len(result.Rows),
			Zodiac:    string(result.Settings.Zodiac),
			Bodies:    result.Settings.BodyNames(),
			Ephemeris: "Swiss Ephemeris " + s.engine.EphemerisVersion(),
			Source:    sourceURL,
		},
	}
	if result.Settings.Zodiac == astro.ZodiacSidereal {
		resp.Meta.Ayanamsha = result.Settings.Ayanamsha.Name
	}
	for _, row := range result.Rows {
		resp.Rows = append(resp.Rows, ephemerisRow{
			UTC:         row.UTC.Format(rfc3339),
			JulianDayUT: row.JulianDayUT,
			Bodies:      row.Positions,
		})
	}
	if len(resp.Rows) > 0 {
		resp.Meta.From = resp.Rows[0].UTC
		resp.Meta.To = resp.Rows[len(resp.Rows)-1].UTC
	}

	writeJSON(w, r, http.StatusOK, resp)
}

// moonPhasesResponse lists the quarters of the lunar cycle in a span.
type moonPhasesResponse struct {
	Phases []moonPhaseEntry `json:"phases"`
	Meta   listMeta         `json:"meta"`
}

type moonPhaseEntry struct {
	Phase string `json:"phase"`
	UTC   string `json:"utc"`

	// Moon and Sun are where the two stood. At a new moon they are together
	// and at a full moon opposite, which is what the phase is.
	Moon astro.Position `json:"moon"`
	Sun  astro.Position `json:"sun"`
}

// listMeta describes a search over a span.
type listMeta struct {
	Count     int    `json:"count"`
	Ephemeris string `json:"ephemeris"`
	Source    string `json:"source"`
}

func (s *Server) handleMoonPhases(w http.ResponseWriter, r *http.Request) {
	var req astro.RangeRequest
	if p := decodeJSON(r, &req); p != nil {
		WriteProblem(w, r, p)
		return
	}

	phases, err := s.engine.MoonPhases(r.Context(), req)
	if err != nil {
		WriteProblem(w, r, problemFromFieldError(err))
		return
	}

	resp := moonPhasesResponse{
		Phases: make([]moonPhaseEntry, 0, len(phases)),
		Meta:   s.listMeta(len(phases)),
	}
	for _, p := range phases {
		resp.Phases = append(resp.Phases, moonPhaseEntry{
			Phase: string(p.Phase),
			UTC:   p.UTC.Format(rfc3339),
			Moon:  p.Moon,
			Sun:   p.Sun,
		})
	}
	writeJSON(w, r, http.StatusOK, resp)
}

// retrogradesResponse lists the stretches of backward motion in a span.
type retrogradesResponse struct {
	Periods []retrogradeEntry `json:"periods"`
	Meta    listMeta          `json:"meta"`
}

type retrogradeEntry struct {
	Body string `json:"body"`

	// Begins and Ends are absent when the period runs past the edge of the
	// span searched, rather than being filled in with a made up date.
	Begins *stationEntry `json:"begins,omitempty"`
	Ends   *stationEntry `json:"ends,omitempty"`

	Days float64 `json:"days,omitempty"`
}

type stationEntry struct {
	UTC      string         `json:"utc"`
	Position astro.Position `json:"position"`
}

func (s *Server) handleRetrogrades(w http.ResponseWriter, r *http.Request) {
	var req astro.RetrogradeRequest
	if p := decodeJSON(r, &req); p != nil {
		WriteProblem(w, r, p)
		return
	}

	periods, err := s.engine.Retrogrades(r.Context(), req)
	if err != nil {
		WriteProblem(w, r, problemFromFieldError(err))
		return
	}

	resp := retrogradesResponse{
		Periods: make([]retrogradeEntry, 0, len(periods)),
		Meta:    s.listMeta(len(periods)),
	}
	for _, p := range periods {
		resp.Periods = append(resp.Periods, retrogradeEntry{
			Body:   p.Body,
			Begins: stationOf(p.Begins),
			Ends:   stationOf(p.Ends),
			Days:   p.Days,
		})
	}
	writeJSON(w, r, http.StatusOK, resp)
}

func stationOf(st *astro.Station) *stationEntry {
	if st == nil {
		return nil
	}
	return &stationEntry{UTC: st.UTC.Format(rfc3339), Position: st.Position}
}

// eclipsesResponse lists the eclipses in a span.
type eclipsesResponse struct {
	Eclipses []eclipseEntry `json:"eclipses"`
	Meta     listMeta       `json:"meta"`
}

type eclipseEntry struct {
	Kind string `json:"kind"`
	Type string `json:"type"`

	// Central is whether the axis of the Moon's shadow meets the Earth. It
	// only means anything for a solar eclipse.
	Central bool `json:"central,omitempty"`

	MaximumUTC string `json:"maximum_utc"`

	Sun  astro.Position `json:"sun"`
	Moon astro.Position `json:"moon"`

	// GreatestAt is where a solar eclipse is deepest and how much of the Sun
	// is covered there. A lunar eclipse looks the same from everywhere it can
	// be seen, so it carries none of this.
	GreatestAt  *astro.Location `json:"greatest_at,omitempty"`
	Magnitude   float64         `json:"magnitude,omitempty"`
	Obscuration float64         `json:"obscuration,omitempty"`
}

func (s *Server) handleEclipses(w http.ResponseWriter, r *http.Request) {
	var req astro.EclipseRequest
	if p := decodeJSON(r, &req); p != nil {
		WriteProblem(w, r, p)
		return
	}

	eclipses, err := s.engine.Eclipses(r.Context(), req)
	if err != nil {
		WriteProblem(w, r, problemFromFieldError(err))
		return
	}

	resp := eclipsesResponse{
		Eclipses: make([]eclipseEntry, 0, len(eclipses)),
		Meta:     s.listMeta(len(eclipses)),
	}
	for _, ec := range eclipses {
		resp.Eclipses = append(resp.Eclipses, eclipseEntry{
			Kind:        string(ec.Kind),
			Type:        ec.Type,
			Central:     ec.Central,
			MaximumUTC:  ec.MaximumUTC.Format(rfc3339),
			Sun:         ec.Sun,
			Moon:        ec.Moon,
			GreatestAt:  ec.GreatestAt,
			Magnitude:   ec.Magnitude,
			Obscuration: ec.Obscuration,
		})
	}
	writeJSON(w, r, http.StatusOK, resp)
}

// riseSetResponse lists when bodies cross the horizon and the meridian.
type riseSetResponse struct {
	Location astro.Location `json:"location"`
	Days     []riseSetDay   `json:"days"`
	Notes    []string       `json:"notes,omitempty"`
	Meta     listMeta       `json:"meta"`
}

type riseSetDay struct {
	Date   string          `json:"date"`
	Bodies []riseSetBodies `json:"bodies"`
}

type riseSetBodies struct {
	Body string `json:"body"`

	// Any of these can be absent. Inside the polar circles a body can stay
	// above or below the horizon for weeks, and then it neither rises nor
	// sets.
	Rise        *string `json:"rise,omitempty"`
	Set         *string `json:"set,omitempty"`
	Culmination *string `json:"culmination,omitempty"`
	Nadir       *string `json:"nadir,omitempty"`
}

func (s *Server) handleRiseSet(w http.ResponseWriter, r *http.Request) {
	var req astro.RiseSetRequest
	if p := decodeJSON(r, &req); p != nil {
		WriteProblem(w, r, p)
		return
	}

	result, err := s.engine.RiseSet(r.Context(), req)
	if err != nil {
		WriteProblem(w, r, problemFromFieldError(err))
		return
	}

	resp := riseSetResponse{
		Location: result.Location,
		Days:     make([]riseSetDay, 0, len(result.Days)),
		Notes:    result.Notes,
		Meta:     s.listMeta(len(result.Days)),
	}
	for _, d := range result.Days {
		day := riseSetDay{Date: d.Date, Bodies: make([]riseSetBodies, 0, len(d.Bodies))}
		for _, b := range d.Bodies {
			day.Bodies = append(day.Bodies, riseSetBodies{
				Body:        b.Body,
				Rise:        formatTime(b.Rise),
				Set:         formatTime(b.Set),
				Culmination: formatTime(b.Culmination),
				Nadir:       formatTime(b.Nadir),
			})
		}
		resp.Days = append(resp.Days, day)
	}
	writeJSON(w, r, http.StatusOK, resp)
}

// formatTime renders an optional moment, keeping absent absent.
func formatTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(rfc3339)
	return &s
}

// listMeta fills in the parts every span search reports the same way.
func (s *Server) listMeta(count int) listMeta {
	return listMeta{
		Count:     count,
		Ephemeris: "Swiss Ephemeris " + s.engine.EphemerisVersion(),
		Source:    sourceURL,
	}
}
