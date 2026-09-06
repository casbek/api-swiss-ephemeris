package astro

import (
	"context"
	"fmt"

	"github.com/casbek/api-swiss-ephemeris/internal/libswe"
	"github.com/casbek/api-swiss-ephemeris/internal/swe"
	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// returnable lists the bodies a return chart can be cast for, with the search
// parameters each needs.
//
// The step has to be short enough that the body cannot pass its natal degree
// and come back within one step. The window has to be long enough to contain a
// full revolution whatever the starting point.
var returnable = map[string]struct {
	SwissID int
	Period  float64 // days for one revolution
	Step    float64 // days between samples
}{
	"sun":  {libswe.Sun, 365.2422, 1.0},
	"moon": {libswe.Moon, 27.32158, 0.25},
}

// maxReturns bounds how many successive returns one request may ask for. A
// year of lunar returns is thirteen, so two dozen covers any sensible use and
// stops a single request from running the ephemeris for a decade.
const maxReturns = 24

// ReturnRequest asks when a body next comes back to the degree it held at
// birth, and for the chart of that moment.
type ReturnRequest struct {
	Natal ChartRequest `json:"natal"`

	// Body is sun for a solar return or moon for a lunar one.
	Body string `json:"body"`

	// From is the moment to search forward from. The first return at or
	// after it is the one returned.
	From tz.Input `json:"from"`

	// Location is where to cast the return chart. It defaults to the natal
	// place. Casting it for where the person actually is at the time is the
	// other common practice, which is why it can be given.
	Location *Location `json:"location,omitempty"`

	// Count is how many successive returns to compute. It defaults to one.
	Count int `json:"count,omitempty"`

	Settings *SettingsInput `json:"settings,omitempty"`
}

// ReturnChart is one return: the moment the body came back and the chart of
// that moment.
type ReturnChart struct {
	Chart *Chart

	// NatalLongitude is the degree the body is returning to.
	NatalLongitude float64
}

// ReturnResult is a birth chart and the returns found for it.
type ReturnResult struct {
	Body  string
	Natal *Chart

	// SearchedFrom is the moment the search started at. The first return is
	// the first one at or after it.
	SearchedFrom Moment

	Returns []ReturnChart
}

// Returns finds when a body comes back to its natal degree and casts the chart
// of each such moment.
func (e *Engine) Returns(ctx context.Context, req ReturnRequest) (*ReturnResult, error) {
	spec, ok := returnable[req.Body]
	if !ok {
		return nil, &tz.FieldError{
			Field:   "body",
			Message: fmt.Sprintf("%q has no return chart; use sun or moon", req.Body),
		}
	}

	count := req.Count
	if count == 0 {
		count = 1
	}
	if count < 1 || count > maxReturns {
		return nil, &tz.FieldError{
			Field:   "count",
			Message: fmt.Sprintf("must be between 1 and %d, got %d", maxReturns, count),
		}
	}

	natal, err := e.Cast(ctx, req.Natal)
	if err != nil {
		return nil, prefixField(err, "natal")
	}

	from, err := NewMoment(req.From)
	if err != nil {
		return nil, prefixField(err, "from")
	}

	settings := natal.Settings
	if req.Settings != nil {
		if settings, err = ResolveSettings(*req.Settings, req.Natal.Location); err != nil {
			return nil, err
		}
	}

	location := req.Natal.Location
	if req.Location != nil {
		location = *req.Location
		if err := location.Validate(); err != nil {
			return nil, prefixField(err, "return")
		}
		if err := location.CheckHouseSystem(settings.HouseSystem); err != nil {
			return nil, err
		}
	}

	// The natal longitude to return to, and the moments of the returns, all
	// come from one session so they are read consistently.
	var natalLongitude float64
	moments := make([]float64, 0, count)

	opts := settings.sweOptions(location)
	err = e.calc.Do(ctx, func(s *swe.Session) error {
		longitudeAt := func(jd float64) (float64, float64, error) {
			res, err := s.Calc(jd, spec.SwissID, opts)
			return res.Longitude, res.SpeedLong, err
		}

		natalRes, err := s.Calc(natal.Moment.JulianDayUT, spec.SwissID, opts)
		if err != nil {
			return err
		}
		natalLongitude = natalRes.Longitude

		search := from.JulianDayUT
		for i := 0; i < count; i++ {
			jd, found, err := crossing(longitudeAt, natalLongitude, search,
				spec.Period*1.05, spec.Step)
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("no %s return was found within %.0f days of the start",
					req.Body, spec.Period*1.05)
			}
			moments = append(moments, jd)
			// Start the next search just past this return, so the same
			// crossing is not found again.
			search = jd + spec.Period*0.5
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	result := &ReturnResult{
		Body:         req.Body,
		Natal:        natal,
		SearchedFrom: from,
		Returns:      make([]ReturnChart, 0, len(moments)),
	}
	for _, jd := range moments {
		moment, err := MomentFromJD(jd)
		if err != nil {
			return nil, err
		}
		chart, err := e.cast(ctx, moment, location, settings)
		if err != nil {
			return nil, err
		}
		result.Returns = append(result.Returns, ReturnChart{
			Chart:          chart,
			NatalLongitude: natalLongitude,
		})
	}
	return result, nil
}
