package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/casbek/api-swiss-ephemeris/internal/astro"
	"github.com/casbek/api-swiss-ephemeris/internal/swe"
	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// timeRequest asks what instant a local reading corresponds to.
type timeRequest struct {
	DateTime tz.Input        `json:"datetime"`
	Location *astro.Location `json:"location,omitempty"`
}

// timeResponse reports the resolved instant and how it was arrived at.
//
// This endpoint exists because time zone handling is where results most often
// disagree between services. Rather than leaving a client to guess, it lets
// them check the conversion on its own, with the reasoning visible.
type timeResponse struct {
	UTC   string       `json:"utc"`
	Local string       `json:"local"`
	Zone  tz.Zone      `json:"timezone"`
	Julia julianDays   `json:"julian_day"`
	Flags timeFlags    `json:"flags"`
	Notes []string     `json:"notes,omitempty"`
	Meta  responseMeta `json:"meta"`
}

type julianDays struct {
	UT float64 `json:"ut"`
	TT float64 `json:"tt"`
}

type timeFlags struct {
	// TimeKnown is false when no time was sent and midday was assumed.
	TimeKnown bool `json:"time_known"`

	// Anomaly is "nonexistent" when the clocks jumped over the reading and
	// "ambiguous" when they went back over it. Empty otherwise.
	Anomaly tz.Anomaly `json:"anomaly,omitempty"`

	// Candidates lists both instants when the reading is ambiguous.
	Candidates []string `json:"candidates,omitempty"`
}

// responseMeta describes how a result was produced. Every calculation endpoint
// carries one, so a client comparing results with another service can see the
// settings rather than guess at them.
type responseMeta struct {
	Ephemeris     string  `json:"ephemeris"`
	DeltaTSeconds float64 `json:"delta_t_seconds"`
	Source        string  `json:"source"`
}

// handleTime resolves a local date and time into an instant.
func (s *Server) handleTime(w http.ResponseWriter, r *http.Request) {
	var req timeRequest
	if p := decodeJSON(r, &req); p != nil {
		WriteProblem(w, r, p)
		return
	}

	if req.Location != nil {
		if err := req.Location.Validate(); err != nil {
			WriteProblem(w, r, problemFromFieldError(err))
			return
		}
	}

	moment, err := astro.NewMoment(req.DateTime)
	if err != nil {
		WriteProblem(w, r, problemFromFieldError(err))
		return
	}

	// Delta T is the gap between Terrestrial Time and Universal Time. It has
	// to be read on a worker thread like any other ephemeris value.
	var deltaT float64
	if err := s.calc.Do(r.Context(), func(sess *swe.Session) error {
		d, err := sess.DeltaT(moment.JulianDayTT)
		if err != nil {
			return err
		}
		deltaT = d * 86400 // the library reports days
		return nil
	}); err != nil {
		s.log.Error("could not read delta T",
			slog.String("request_id", RequestIDFrom(r.Context())),
			slog.String("error", err.Error()))
		WriteProblem(w, r, Internal())
		return
	}

	resp := timeResponse{
		UTC:   moment.UTC.Format(time.RFC3339),
		Local: moment.Local.Format(time.RFC3339),
		Zone:  moment.Zone,
		Julia: julianDays{UT: moment.JulianDayUT, TT: moment.JulianDayTT},
		Flags: timeFlags{
			TimeKnown: moment.TimeKnown,
			Anomaly:   moment.Anomaly,
		},
		Meta: responseMeta{
			Ephemeris:     "Swiss Ephemeris " + s.calc.Version(),
			DeltaTSeconds: deltaT,
			Source:        sourceURL,
		},
	}

	for _, alt := range moment.Alternatives {
		resp.Flags.Candidates = append(resp.Flags.Candidates, alt.Format(time.RFC3339))
	}
	resp.Notes = notesFor(moment)

	writeJSON(w, r, http.StatusOK, resp)
}

// notesFor explains anything about the result a client should act on.
func notesFor(m astro.Moment) []string {
	var notes []string

	if !m.TimeKnown {
		notes = append(notes,
			"No time was given, so midday local time was assumed. Houses and angles "+
				"cannot be calculated for a chart with an unknown birth time.")
	}

	switch m.Anomaly {
	case tz.AnomalyNonexistent:
		notes = append(notes,
			"The clocks went forward over this reading, so it never occurred. The "+
				"instant reported is the one the clock showed after the change. If the "+
				"recorded time is trusted, check whether it was written in standard time.")
	case tz.AnomalyAmbiguous:
		notes = append(notes,
			"The clocks went back over this reading, so it occurred twice. The later "+
				"of the two was used. Send utc_offset instead of timezone to choose "+
				"which one you mean.")
	}

	if m.Zone.Source == tz.SourceFixedOffset {
		notes = append(notes,
			"A fixed offset was given, so no daylight saving rule was applied. Send an "+
				"IANA timezone name to have the rules of the date applied.")
	}
	return notes
}

// problemFromFieldError turns a validation failure into a 422 that names the
// field, so a client can correct the request without guessing.
func problemFromFieldError(err error) *Problem {
	var fe *tz.FieldError
	if errors.As(err, &fe) {
		return Validation("The request could not be turned into a moment in time.",
			FieldError{Field: fe.Field, Message: fe.Message})
	}
	return Validation(err.Error())
}
