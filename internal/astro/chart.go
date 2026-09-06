package astro

import (
	"context"
	"fmt"
	"math"

	"github.com/casbek/api-swiss-ephemeris/internal/libswe"
	"github.com/casbek/api-swiss-ephemeris/internal/swe"
	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// Engine calculates charts.
type Engine struct {
	calc *swe.Calculator
}

// NewEngine wraps a calculator.
func NewEngine(calc *swe.Calculator) *Engine { return &Engine{calc: calc} }

// EphemerisVersion reports the version of the underlying library.
func (e *Engine) EphemerisVersion() string { return e.calc.Version() }

// ChartRequest is everything needed to cast a chart.
type ChartRequest struct {
	DateTime tz.Input      `json:"datetime"`
	Location Location      `json:"location"`
	Settings SettingsInput `json:"settings,omitempty"`
}

// Chart is a cast chart.
type Chart struct {
	Moment   Moment
	Location Location
	Settings Settings

	// Sect is whether the chart is a day or a night one. It is empty when
	// the birth time is unknown, since it depends on the houses.
	Sect Sect

	Positions    []Position
	Houses       *Houses
	Aspects      []Aspect
	Distribution Distribution

	// Ayanamsha is the offset applied, in degrees. Only set for a sidereal
	// chart.
	Ayanamsha float64

	// Obliquity is the tilt of the Earth's axis at this moment, in degrees.
	Obliquity float64
}

// Cast computes a chart.
//
// Every ephemeris call happens inside one session, so the whole chart comes
// from a single consistent pass on one thread rather than from values gathered
// piecemeal.
func (e *Engine) Cast(ctx context.Context, req ChartRequest) (*Chart, error) {
	if err := req.Location.Validate(); err != nil {
		return nil, err
	}

	moment, err := NewMoment(req.DateTime)
	if err != nil {
		return nil, err
	}

	settings, err := ResolveSettings(req.Settings, req.Location)
	if err != nil {
		return nil, err
	}

	chart := &Chart{
		Moment:   moment,
		Location: req.Location,
		Settings: settings,
	}

	opts := settings.sweOptions(req.Location)
	jd := moment.JulianDayUT

	err = e.calc.Do(ctx, func(s *swe.Session) error {
		// The obliquity is needed to derive declinations, and is worth
		// reporting in its own right.
		eclNut, err := s.Calc(jd, libswe.EclNut, swe.Options{})
		if err != nil {
			return fmt.Errorf("obliquity: %w", err)
		}
		chart.Obliquity = eclNut.Longitude

		if settings.Zodiac == ZodiacSidereal {
			if chart.Ayanamsha, err = s.Ayanamsha(jd, opts); err != nil {
				return fmt.Errorf("ayanamsha: %w", err)
			}
		}

		// Houses depend entirely on the time of day. With an unknown birth
		// time they would be invention, so they are left out.
		if moment.TimeKnown {
			raw, err := s.Houses(jd, req.Location.Latitude, req.Location.Longitude,
				settings.HouseSystem.Letter, opts)
			if err != nil {
				return fmt.Errorf("houses: %w", err)
			}
			chart.Houses = buildHouses(settings.HouseSystem, raw.Cusps, raw.ASCMC)
		}

		chart.Positions, err = e.positions(s, jd, settings, opts, chart.Obliquity)
		return err
	})
	if err != nil {
		return nil, err
	}

	// Houses, sect and dignity depend on each other in that order, so they
	// are settled after every position is known.
	if chart.Houses != nil {
		for i := range chart.Positions {
			chart.Houses.assign(&chart.Positions[i])
		}
		chart.Sect = SectOf(sunHouse(chart.Positions))
	}

	for i := range chart.Positions {
		p := &chart.Positions[i]
		p.Dignity = DignityOf(p.Body, p.Longitude, chart.Sect)
	}

	// The angles take part in aspects like any other point, but they are
	// reported with the houses rather than among the bodies.
	aspectable := chart.Positions
	if chart.Houses != nil {
		aspectable = append(append([]Position(nil), chart.Positions...), chart.Houses.Angles...)
	}
	chart.Aspects = FindAspects(aspectable, settings.AspectTypes, settings.Orbs)
	chart.Distribution = Distribute(chart.Positions)

	return chart, nil
}

// positions computes every requested body.
func (e *Engine) positions(s *swe.Session, jd float64, settings Settings, opts swe.Options, obliquity float64) ([]Position, error) {
	out := make([]Position, 0, len(settings.Bodies))

	for _, body := range settings.Bodies {
		res, err := e.rawPosition(s, jd, body, opts)
		if err != nil {
			return nil, err
		}

		p := newPosition(body.Name, body.Label, res.Longitude)
		p.Latitude = res.Latitude
		p.DistanceAU = res.Distance
		p.Speed = res.SpeedLong
		p.IsRetrograde = res.SpeedLong < 0
		p.Declination = declination(p.Longitude, res.Latitude, obliquity)
		out = append(out, p)
	}
	return out, nil
}

// rawPosition asks the ephemeris for a body, deriving the ones that are not
// computed directly.
func (e *Engine) rawPosition(s *swe.Session, jd float64, body Body, opts swe.Options) (libswe.CalcResult, error) {
	if !body.Derived() {
		return s.Calc(jd, body.SwissID, opts)
	}

	switch body.Name {
	case "south_node":
		// The south node is always exactly opposite the north node, and
		// shares its motion.
		res, err := s.Calc(jd, libswe.TrueNode, opts)
		if err != nil {
			return libswe.CalcResult{}, err
		}
		res.Longitude = Normalize(res.Longitude + 180)
		res.Latitude = -res.Latitude
		return res, nil
	default:
		return libswe.CalcResult{}, fmt.Errorf("astro: %s has no calculation", body.Name)
	}
}

// declination converts an ecliptic position to its declination, the angle
// north or south of the celestial equator.
//
// Deriving it from the longitude, latitude and obliquity avoids asking the
// ephemeris a second time for every body, and gives the same answer.
func declination(longitude, latitude, obliquity float64) float64 {
	lon := longitude * math.Pi / 180
	lat := latitude * math.Pi / 180
	eps := obliquity * math.Pi / 180

	sinDec := math.Sin(lat)*math.Cos(eps) + math.Cos(lat)*math.Sin(eps)*math.Sin(lon)
	return math.Asin(sinDec) * 180 / math.Pi
}

// sunHouse finds which house the Sun is in, or 0 if the Sun was not requested.
func sunHouse(positions []Position) int {
	for _, p := range positions {
		if p.Body == "sun" {
			return p.House
		}
	}
	return 0
}
