package astro

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/casbek/api-swiss-ephemeris/internal/libswe"
	"github.com/casbek/api-swiss-ephemeris/internal/swe"
	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// Limits on how much sky one request may ask for. They are not about the cost
// of the calculation, which is small, but about a response staying something a
// client can hold and read.
const (
	maxEphemerisRows = 1000
	maxMoonPhases    = 400
	maxStations      = 200
	maxEclipses      = 200

	// maxSearchYears bounds the span an event search may cover.
	maxSearchYears = 50
)

// synodicMonth is the average time from one new moon to the next, in days.
const synodicMonth = 29.530588

// RangeRequest is a span of time to look at.
type RangeRequest struct {
	From tz.Input `json:"from"`
	To   tz.Input `json:"to"`
}

// resolveRange turns a span into two moments, checking it makes sense.
func resolveRange(r RangeRequest) (from, to Moment, err error) {
	if from, err = NewMoment(r.From); err != nil {
		return Moment{}, Moment{}, prefixField(err, "from")
	}
	if to, err = NewMoment(r.To); err != nil {
		return Moment{}, Moment{}, prefixField(err, "to")
	}
	if to.JulianDayUT <= from.JulianDayUT {
		return Moment{}, Moment{}, &tz.FieldError{
			Field:   "to",
			Message: "must be after from",
		}
	}
	if days := to.JulianDayUT - from.JulianDayUT; days > maxSearchYears*tropicalYear {
		return Moment{}, Moment{}, &tz.FieldError{
			Field: "to",
			Message: fmt.Sprintf("the span is %.0f years; at most %d may be searched at once",
				days/tropicalYear, maxSearchYears),
		}
	}
	return from, to, nil
}

// EphemerisRequest asks for positions at regular intervals.
type EphemerisRequest struct {
	RangeRequest

	// StepDays is the interval between rows. It defaults to one day.
	StepDays float64 `json:"step_days,omitempty"`

	Settings SettingsInput `json:"settings,omitempty"`
}

// EphemerisRow is the sky at one moment.
type EphemerisRow struct {
	UTC         time.Time
	JulianDayUT float64
	Positions   []Position
}

// EphemerisResult is a table of positions.
type EphemerisResult struct {
	Rows     []EphemerisRow
	Settings Settings
	StepDays float64
}

// Ephemeris tabulates positions at regular intervals.
//
// Houses are not part of it: they depend on a place as well as a moment, and
// an ephemeris is a table of where the bodies are, not of how they fall for
// anyone in particular.
func (e *Engine) Ephemeris(ctx context.Context, req EphemerisRequest) (*EphemerisResult, error) {
	from, to, err := resolveRange(req.RangeRequest)
	if err != nil {
		return nil, err
	}

	step := req.StepDays
	if step == 0 {
		step = 1
	}
	if step <= 0 {
		return nil, &tz.FieldError{
			Field:   "step_days",
			Message: fmt.Sprintf("must be positive, got %g", step),
		}
	}

	span := to.JulianDayUT - from.JulianDayUT
	rows := int(span/step) + 1
	if rows > maxEphemerisRows {
		return nil, &tz.FieldError{
			Field: "step_days",
			Message: fmt.Sprintf("a step of %g days over this span gives %d rows; "+
				"at most %d are returned, so use a wider step or a shorter span",
				step, rows, maxEphemerisRows),
		}
	}

	// An ephemeris has no place, so the settings are resolved against the
	// equator, where every house system is defined. Houses are not used.
	settings, err := ResolveSettings(req.Settings, Location{})
	if err != nil {
		return nil, err
	}

	result := &EphemerisResult{
		Settings: settings,
		StepDays: step,
		Rows:     make([]EphemerisRow, 0, rows),
	}

	opts := settings.sweOptions(Location{})
	err = e.calc.Do(ctx, func(s *swe.Session) error {
		for jd := from.JulianDayUT; jd <= to.JulianDayUT+1e-9; jd += step {
			eclNut, err := s.Calc(jd, libswe.EclNut, swe.Options{})
			if err != nil {
				return err
			}

			positions, err := e.positions(s, jd, settings, opts, eclNut.Longitude)
			if err != nil {
				return err
			}
			for i := range positions {
				positions[i].Dignity = nil // no sect without a place and a time of day
			}

			moment, err := MomentFromJD(jd)
			if err != nil {
				return err
			}
			result.Rows = append(result.Rows, EphemerisRow{
				UTC:         moment.UTC,
				JulianDayUT: jd,
				Positions:   positions,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// MoonPhaseKind names one of the four quarters of the lunar cycle.
type MoonPhaseKind string

const (
	PhaseNew          MoonPhaseKind = "new_moon"
	PhaseFirstQuarter MoonPhaseKind = "first_quarter"
	PhaseFull         MoonPhaseKind = "full_moon"
	PhaseLastQuarter  MoonPhaseKind = "last_quarter"
)

// phaseTargets are the elongations that define each quarter: how far the Moon
// stands ahead of the Sun.
var phaseTargets = []struct {
	Kind       MoonPhaseKind
	Elongation float64
}{
	{PhaseNew, 0},
	{PhaseFirstQuarter, 90},
	{PhaseFull, 180},
	{PhaseLastQuarter, 270},
}

// MoonPhase is one quarter of the lunar cycle.
type MoonPhase struct {
	Phase       MoonPhaseKind
	UTC         time.Time
	JulianDayUT float64

	Moon Position
	Sun  Position
}

// MoonPhases finds every new, first quarter, full and last quarter moon in a
// span.
//
// The phase is the angle between the Moon and the Sun, so each one is found by
// solving for when that angle reaches a quarter of the circle. It closes on
// each moment to within a tenth of a second, which is finer than the boundary
// between one phase and the next is ever quoted.
func (e *Engine) MoonPhases(ctx context.Context, req RangeRequest) ([]MoonPhase, error) {
	from, to, err := resolveRange(req)
	if err != nil {
		return nil, err
	}

	var found []MoonPhase

	err = e.calc.Do(ctx, func(s *swe.Session) error {
		// The elongation behaves like a longitude of its own: it runs from 0
		// to 360 once a lunar month, at the rate the Moon gains on the Sun.
		elongation := func(jd float64) (float64, float64, error) {
			moon, err := s.Calc(jd, libswe.Moon, swe.Options{})
			if err != nil {
				return 0, 0, err
			}
			sun, err := s.Calc(jd, libswe.Sun, swe.Options{})
			if err != nil {
				return 0, 0, err
			}
			return Normalize(moon.Longitude - sun.Longitude),
				moon.SpeedLong - sun.SpeedLong, nil
		}

		for _, target := range phaseTargets {
			search := from.JulianDayUT
			for len(found) < maxMoonPhases {
				// A quarter of a day is a sixth of the elongation's daily
				// gain, so no crossing can be stepped over.
				jd, ok, err := crossing(elongation, target.Elongation, search,
					synodicMonth*1.05, 0.25)
				if err != nil {
					return err
				}
				if !ok || jd > to.JulianDayUT {
					break
				}

				phase, err := e.describePhase(s, target.Kind, jd)
				if err != nil {
					return err
				}
				found = append(found, phase)

				// Move past this one; the next is a lunar month away.
				search = jd + synodicMonth*0.5
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(found, func(i, j int) bool {
		return found[i].JulianDayUT < found[j].JulianDayUT
	})
	return found, nil
}

// describePhase fills in where the Moon and Sun stood at a phase.
func (e *Engine) describePhase(s *swe.Session, kind MoonPhaseKind, jd float64) (MoonPhase, error) {
	moment, err := MomentFromJD(jd)
	if err != nil {
		return MoonPhase{}, err
	}

	moon, err := s.Calc(jd, libswe.Moon, swe.Options{})
	if err != nil {
		return MoonPhase{}, err
	}
	sun, err := s.Calc(jd, libswe.Sun, swe.Options{})
	if err != nil {
		return MoonPhase{}, err
	}

	moonPos := newPosition("moon", "Moon", moon.Longitude)
	moonPos.Latitude, moonPos.Speed = moon.Latitude, moon.SpeedLong
	sunPos := newPosition("sun", "Sun", sun.Longitude)
	sunPos.Latitude, sunPos.Speed = sun.Latitude, sun.SpeedLong

	return MoonPhase{
		Phase:       kind,
		UTC:         moment.UTC,
		JulianDayUT: jd,
		Moon:        moonPos,
		Sun:         sunPos,
	}, nil
}

// StationKind is which way a body turns at a station.
type StationKind string

const (
	// StationRetrograde is where a body stops moving forward and begins to
	// go back.
	StationRetrograde StationKind = "retrograde"
	// StationDirect is where it stops going back and resumes.
	StationDirect StationKind = "direct"
)

// Station is a moment a body's motion reverses.
type Station struct {
	Kind        StationKind
	UTC         time.Time
	JulianDayUT float64
	Position    Position
}

// RetrogradePeriod is a stretch of backward motion.
type RetrogradePeriod struct {
	Body string

	// Begins and Ends are the stations that open and close the period. Either
	// is absent when the period runs past the edge of the span searched.
	Begins *Station
	Ends   *Station

	// Days is the length of the period, when both ends are known.
	Days float64
}

// RetrogradeRequest asks when a body moves backwards.
type RetrogradeRequest struct {
	RangeRequest

	// Bodies defaults to the planets that visibly retrograde, which is every
	// one from Mercury outwards.
	Bodies []string `json:"bodies,omitempty"`
}

// defaultRetrogradeBodies are the planets whose retrograde periods are watched.
// The Sun and Moon never turn back, and the nodes are a different matter.
var defaultRetrogradeBodies = []string{
	"mercury", "venus", "mars", "jupiter", "saturn", "uranus", "neptune", "pluto",
}

// Retrogrades finds the stretches of backward motion in a span.
//
// A station is where a body's speed passes through zero, so it is found by
// solving on the speed rather than on the position. A period that is already
// under way when the span opens, or still running when it closes, is reported
// with the missing end absent rather than with a made up date.
func (e *Engine) Retrogrades(ctx context.Context, req RetrogradeRequest) ([]RetrogradePeriod, error) {
	from, to, err := resolveRange(req.RangeRequest)
	if err != nil {
		return nil, err
	}

	names := req.Bodies
	if len(names) == 0 {
		names = defaultRetrogradeBodies
	}

	bodies := make([]Body, 0, len(names))
	for _, name := range names {
		body, ok := LookupBody(name)
		if !ok {
			return nil, &tz.FieldError{
				Field:   "bodies",
				Message: fmt.Sprintf("%q is not a known body; see /v1/reference/bodies", name),
			}
		}
		if body.Name == "sun" || body.Name == "moon" {
			return nil, &tz.FieldError{
				Field:   "bodies",
				Message: fmt.Sprintf("the %s never moves backwards", body.Name),
			}
		}
		if body.Derived() {
			return nil, &tz.FieldError{
				Field:   "bodies",
				Message: fmt.Sprintf("%q is derived from another point and has no motion of its own", name),
			}
		}
		bodies = append(bodies, body)
	}

	var periods []RetrogradePeriod

	err = e.calc.Do(ctx, func(s *swe.Session) error {
		for _, body := range bodies {
			speedAt := func(jd float64) (float64, error) {
				res, err := s.Calc(jd, body.SwissID, swe.Options{})
				return res.SpeedLong, err
			}

			opening, err := s.Calc(from.JulianDayUT, body.SwissID, swe.Options{})
			if err != nil {
				return err
			}

			stations, err := e.stationsOf(s, body, speedAt, from.JulianDayUT, to.JulianDayUT)
			if err != nil {
				return err
			}
			periods = append(periods,
				pairStations(body.Name, stations, opening.SpeedLong < 0)...)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.SliceStable(periods, func(i, j int) bool {
		return periodStart(periods[i]) < periodStart(periods[j])
	})
	return periods, nil
}

// periodStart is the moment a period begins. One that was already under way
// when the span opened sorts before everything, since it started earlier than
// anything the search saw.
func periodStart(p RetrogradePeriod) float64 {
	if p.Begins != nil {
		return p.Begins.JulianDayUT
	}
	if p.Ends != nil {
		return p.Ends.JulianDayUT - 1e6
	}
	return math.Inf(-1)
}

// stationsOf collects every station of a body within a span.
func (e *Engine) stationsOf(s *swe.Session, body Body, speedAt scalarFunc, from, to float64) ([]Station, error) {
	var stations []Station

	// A day is short enough to catch even Mercury, whose speed swings from
	// two degrees a day to minus one and a half over about a week.
	const step = 1.0

	search := from
	for search < to && len(stations) < maxStations {
		jd, ok, err := zeroCrossing(speedAt, search, to-search, step)
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}

		res, err := s.Calc(jd, body.SwissID, swe.Options{})
		if err != nil {
			return nil, err
		}

		// Which way the turn goes is read from the motion just after it.
		after, err := speedAt(jd + 1)
		if err != nil {
			return nil, err
		}
		kind := StationDirect
		if after < 0 {
			kind = StationRetrograde
		}

		moment, err := MomentFromJD(jd)
		if err != nil {
			return nil, err
		}
		position := newPosition(body.Name, body.Label, res.Longitude)
		position.Latitude = res.Latitude
		position.Speed = res.SpeedLong
		position.IsRetrograde = res.SpeedLong < 0

		stations = append(stations, Station{
			Kind:        kind,
			UTC:         moment.UTC,
			JulianDayUT: jd,
			Position:    position,
		})

		// Step past this station so the same sign change is not found again.
		search = jd + step
	}
	return stations, nil
}

// pairStations turns a run of stations into periods of backward motion.
//
// The open period is tracked by index rather than by pointer: appending to the
// slice can move it in memory, which would leave a pointer addressing the old
// array and quietly discard the end of the period.
func pairStations(body string, stations []Station, alreadyRetrograde bool) []RetrogradePeriod {
	var periods []RetrogradePeriod
	open := -1

	if alreadyRetrograde {
		// The period was under way before the span opened, so its beginning
		// lies outside what was searched and is left absent.
		periods = append(periods, RetrogradePeriod{Body: body})
		open = 0
	}

	for i := range stations {
		st := &stations[i]

		switch st.Kind {
		case StationRetrograde:
			periods = append(periods, RetrogradePeriod{Body: body, Begins: st})
			open = len(periods) - 1

		case StationDirect:
			if open < 0 {
				// A direct station with nothing open means the backward
				// motion began before the span; record the end alone.
				periods = append(periods, RetrogradePeriod{Body: body, Ends: st})
				continue
			}
			periods[open].Ends = st
			if b := periods[open].Begins; b != nil {
				periods[open].Days = st.JulianDayUT - b.JulianDayUT
			}
			open = -1
		}
	}
	return periods
}
