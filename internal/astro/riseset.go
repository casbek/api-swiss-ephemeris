package astro

import (
	"context"
	"fmt"
	"time"

	"github.com/casbek/api-swiss-ephemeris/internal/libswe"
	"github.com/casbek/api-swiss-ephemeris/internal/swe"
	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// maxRiseSetDays bounds how many days of rising and setting one request may
// ask for.
const maxRiseSetDays = 366

// RiseSetRequest asks when bodies rise, set and cross the meridian at a place.
type RiseSetRequest struct {
	// From is the day to start on. Only its date is used; the search runs
	// from local midnight.
	From tz.Input `json:"from"`

	Location Location `json:"location"`

	// Days is how many days to cover, starting at From. It defaults to one.
	Days int `json:"days,omitempty"`

	// Bodies defaults to the Sun and Moon, which is what the question is
	// nearly always about.
	Bodies []string `json:"bodies,omitempty"`
}

// RiseSetEvents are the times a body crosses the horizon and the meridian on
// one day.
//
// Any of them can be absent. Inside the polar circles a body can stay up or
// stay down for weeks, and then it neither rises nor sets.
type RiseSetEvents struct {
	Body  string
	Label string

	Rise        *time.Time
	Set         *time.Time
	Culmination *time.Time // the highest point, crossing the meridian
	Nadir       *time.Time // the lowest, crossing it below the horizon
}

// RiseSetDay is one day at one place.
type RiseSetDay struct {
	Date   string
	Bodies []RiseSetEvents
}

// RiseSetResult is a run of days.
type RiseSetResult struct {
	Location Location
	Days     []RiseSetDay
	Notes    []string
}

// defaultRiseSetBodies is the Sun and Moon.
var defaultRiseSetBodies = []string{"sun", "moon"}

// RiseSet finds when bodies cross the horizon and the meridian.
func (e *Engine) RiseSet(ctx context.Context, req RiseSetRequest) (*RiseSetResult, error) {
	if err := req.Location.Validate(); err != nil {
		return nil, err
	}

	start, err := NewMoment(req.From)
	if err != nil {
		return nil, prefixField(err, "from")
	}

	days := req.Days
	if days == 0 {
		days = 1
	}
	if days < 1 || days > maxRiseSetDays {
		return nil, &tz.FieldError{
			Field:   "days",
			Message: fmt.Sprintf("must be between 1 and %d, got %d", maxRiseSetDays, days),
		}
	}

	names := req.Bodies
	if len(names) == 0 {
		names = defaultRiseSetBodies
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
		if body.Derived() || body.Category == CategoryAngle {
			return nil, &tz.FieldError{
				Field: "bodies",
				Message: fmt.Sprintf("%q is a calculated point, not something that rises or sets",
					name),
			}
		}
		bodies = append(bodies, body)
	}

	result := &RiseSetResult{Location: req.Location}
	if req.Location.IsHighLatitude() {
		result.Notes = append(result.Notes,
			"This place is inside a polar circle, where a body can stay above or "+
				"below the horizon for weeks at a time. A missing rise or set on a "+
				"given day means exactly that.")
	}

	// The search for each day starts at the beginning of that day in UT.
	firstDay := float64(int(start.JulianDayUT - 0.5))

	err = e.calc.Do(ctx, func(s *swe.Session) error {
		for d := 0; d < days; d++ {
			dayStart := firstDay + float64(d) + 0.5

			moment, err := MomentFromJD(dayStart)
			if err != nil {
				return err
			}
			day := RiseSetDay{
				Date:   moment.UTC.Format("2006-01-02"),
				Bodies: make([]RiseSetEvents, 0, len(bodies)),
			}

			for _, body := range bodies {
				events, err := e.riseSetOf(s, body, dayStart, req.Location)
				if err != nil {
					return err
				}
				day.Bodies = append(day.Bodies, events)
			}
			result.Days = append(result.Days, day)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// riseSetOf finds one body's crossings on one day.
//
// Each is searched for within a day of the start, so an event that falls after
// midnight belongs to the next day rather than being reported twice.
func (e *Engine) riseSetOf(s *swe.Session, body Body, dayStart float64, loc Location) (RiseSetEvents, error) {
	events := RiseSetEvents{Body: body.Name, Label: body.Label}

	for _, want := range []struct {
		flag int32
		into **time.Time
	}{
		{libswe.CalcRise, &events.Rise},
		{libswe.CalcSet, &events.Set},
		{libswe.CalcMTransit, &events.Culmination},
		{libswe.CalcITransit, &events.Nadir},
	} {
		jd, found, err := s.RiseTrans(dayStart, body.SwissID, want.flag,
			loc.Latitude, loc.Longitude, loc.AltitudeM, swe.Options{})
		if err != nil {
			return RiseSetEvents{}, err
		}
		// A crossing more than a day away belongs to another day; reporting
		// it here would put the same event on two consecutive days.
		if !found || jd >= dayStart+1 {
			continue
		}

		moment, err := MomentFromJD(jd)
		if err != nil {
			return RiseSetEvents{}, err
		}
		utc := moment.UTC
		*want.into = &utc
	}
	return events, nil
}
