package astro

import (
	"context"
	"fmt"

	"github.com/casbek/api-swiss-ephemeris/internal/swe"
	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// CompositeMethod selects how a relationship chart is derived. The two are not
// variations on one idea: they answer different questions and routinely
// disagree by degrees.
type CompositeMethod string

const (
	// CompositeMidpoint puts every point halfway between the two charts. It
	// is the older and more widely used method. The result is not a chart of
	// any moment: nothing was ever in the sky in that arrangement.
	CompositeMidpoint CompositeMethod = "midpoint"

	// CompositeDavison casts a real chart for the midpoint in time and in
	// place. Its positions were genuinely in the sky at that instant, which
	// some practitioners prefer and others consider beside the point.
	CompositeDavison CompositeMethod = "davison"
)

// DefaultCompositeMethod is used when a request does not name one.
const DefaultCompositeMethod = CompositeMidpoint

// CompositeRequest asks for a relationship chart from two birth charts.
type CompositeRequest struct {
	ChartA ChartRequest `json:"chart_a"`
	ChartB ChartRequest `json:"chart_b"`

	Method   string         `json:"method,omitempty"`
	Settings *SettingsInput `json:"settings,omitempty"`
}

// CompositeResult is a relationship chart and the two it came from.
type CompositeResult struct {
	Method CompositeMethod

	ChartA *Chart
	ChartB *Chart

	Positions    []Position
	Houses       *Houses
	Aspects      []Aspect
	Distribution Distribution
	Sect         Sect
	Settings     Settings

	// Moment and Location are set only for a Davison chart, which is cast
	// for a real instant at a real place. A midpoint composite has neither.
	Moment   *Moment
	Location *Location

	Notes []string
}

// Composite derives a relationship chart from two birth charts.
func (e *Engine) Composite(ctx context.Context, req CompositeRequest) (*CompositeResult, error) {
	method := CompositeMethod(req.Method)
	if req.Method == "" {
		method = DefaultCompositeMethod
	}
	if method != CompositeMidpoint && method != CompositeDavison {
		return nil, &tz.FieldError{
			Field: "method",
			Message: fmt.Sprintf("%q is not a composite method; use midpoint or davison",
				req.Method),
		}
	}

	a, err := e.Cast(ctx, req.ChartA)
	if err != nil {
		return nil, prefixField(err, "chart_a")
	}
	b, err := e.Cast(ctx, req.ChartB)
	if err != nil {
		return nil, prefixField(err, "chart_b")
	}

	settings := a.Settings
	if req.Settings != nil {
		if settings, err = ResolveSettings(*req.Settings, req.ChartA.Location); err != nil {
			return nil, err
		}
	}

	if method == CompositeDavison {
		return e.davison(ctx, a, b, settings)
	}
	return e.midpointComposite(ctx, a, b, settings)
}

// davison casts a real chart for the midpoint in time and place.
func (e *Engine) davison(ctx context.Context, a, b *Chart, settings Settings) (*CompositeResult, error) {
	midJD := (a.Moment.JulianDayUT + b.Moment.JulianDayUT) / 2

	location := Location{
		Latitude:  meanLatitude(a.Location.Latitude, b.Location.Latitude),
		Longitude: meanLongitude(a.Location.Longitude, b.Location.Longitude),
		AltitudeM: (a.Location.AltitudeM + b.Location.AltitudeM) / 2,
	}
	if err := location.CheckHouseSystem(settings.HouseSystem); err != nil {
		return nil, err
	}

	moment, err := MomentFromJD(midJD)
	if err != nil {
		return nil, err
	}

	chart, err := e.cast(ctx, moment, location, settings)
	if err != nil {
		return nil, err
	}

	res := &CompositeResult{
		Method:       CompositeDavison,
		ChartA:       a,
		ChartB:       b,
		Positions:    chart.Positions,
		Houses:       chart.Houses,
		Aspects:      chart.Aspects,
		Distribution: chart.Distribution,
		Sect:         chart.Sect,
		Settings:     settings,
		Moment:       &chart.Moment,
		Location:     &chart.Location,
	}

	// A Davison chart is only as meaningful as the birth times behind it. If
	// either was unknown, its midpoint carries that uncertainty forward.
	if !a.Moment.TimeKnown || !b.Moment.TimeKnown {
		res.Notes = append(res.Notes,
			"One of the birth times was unknown, so midday was assumed for it. "+
				"The midpoint in time, and therefore the houses of this chart, "+
				"inherit that assumption.")
	}
	return res, nil
}

// midpointComposite puts every point halfway between the two charts.
//
// The houses come from the midpoint of the two charts' sidereal times at the
// midpoint of their latitudes. Halving each cusp individually is the other
// approach in circulation, but it can leave the cusps out of order, since a
// house division is not something that survives being averaged piece by piece.
func (e *Engine) midpointComposite(ctx context.Context, a, b *Chart, settings Settings) (*CompositeResult, error) {
	res := &CompositeResult{
		Method:    CompositeMidpoint,
		ChartA:    a,
		ChartB:    b,
		Settings:  settings,
		Positions: midpointPositions(a.Positions, b.Positions),
	}

	if len(res.Positions) == 0 {
		return nil, &tz.FieldError{
			Field:   "settings.bodies",
			Message: "the two charts have no bodies in common to take a midpoint of",
		}
	}

	// Houses need a sidereal time from each chart, which only exists when
	// the birth time is known.
	if a.Houses != nil && b.Houses != nil {
		location := Location{
			Latitude:  meanLatitude(a.Location.Latitude, b.Location.Latitude),
			Longitude: meanLongitude(a.Location.Longitude, b.Location.Longitude),
		}
		if err := location.CheckHouseSystem(settings.HouseSystem); err != nil {
			return nil, err
		}

		armc := Midpoint(a.Houses.ARMC, b.Houses.ARMC)
		eps := (a.Obliquity + b.Obliquity) / 2

		err := e.calc.Do(ctx, func(s *swe.Session) error {
			raw, err := s.HousesFromARMC(armc, location.Latitude, eps,
				settings.HouseSystem.Letter)
			if err != nil {
				return err
			}
			res.Houses = buildHouses(settings.HouseSystem, raw.Cusps, raw.ASCMC)
			return nil
		})
		if err != nil {
			return nil, err
		}

		if cuspsOutOfOrder(res.Houses.Cusps) {
			// Say so rather than serving a division that cannot be read.
			res.Notes = append(res.Notes,
				"The composite house cusps did not come out in order. This happens "+
					"when the two charts are cast far apart in longitude, and means the "+
					"house division of this composite should not be relied on.")
		}

		for i := range res.Positions {
			res.Houses.assign(&res.Positions[i])
		}
		res.Sect = SectOf(sunHouse(res.Positions))
	} else {
		res.Notes = append(res.Notes,
			"One of the birth times was unknown, so the composite has no houses, "+
				"angles or sect. All of them depend on the time of day.")
	}

	for i := range res.Positions {
		p := &res.Positions[i]
		p.Dignity = DignityOf(p.Body, p.Longitude, res.Sect)
	}

	aspectable := res.Positions
	if res.Houses != nil {
		aspectable = append(append([]Position(nil), res.Positions...), res.Houses.Angles...)
	}
	res.Aspects = FindAspects(aspectable, settings.AspectTypes, settings.Orbs)
	res.Distribution = Distribute(res.Positions)

	res.Notes = append(res.Notes,
		"A midpoint composite is not a chart of any moment: nothing was ever in "+
			"the sky in this arrangement. Use the davison method for a chart that was.")

	return res, nil
}
