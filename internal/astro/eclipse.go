package astro

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/casbek/api-swiss-ephemeris/internal/libswe"
	"github.com/casbek/api-swiss-ephemeris/internal/swe"
	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// EclipseKind is which body is eclipsed.
type EclipseKind string

const (
	EclipseSolar EclipseKind = "solar"
	EclipseLunar EclipseKind = "lunar"
)

// Eclipse is one eclipse.
type Eclipse struct {
	Kind EclipseKind

	// Type is total, annular, hybrid, partial or penumbral. A hybrid eclipse
	// is total along part of its track and annular along the rest.
	Type string

	// Central is true when the axis of the Moon's shadow meets the Earth.
	// Only meaningful for a solar eclipse.
	Central bool

	// MaximumUTC is when the eclipse is greatest.
	MaximumUTC  time.Time
	JulianDayUT float64

	// Sun and Moon are where the two stood at maximum. A solar eclipse has
	// them conjunct and a lunar one opposite, which is what an eclipse is.
	Sun  Position
	Moon Position

	// GreatestAt is where on Earth a solar eclipse is deepest.
	GreatestAt *Location

	// Magnitude is the fraction of the Sun's diameter the Moon covers there.
	// It exceeds one for a total eclipse, where the Moon is the larger of the
	// two discs, and that is meaningful rather than an error.
	Magnitude float64

	// Obscuration is the fraction of the Sun's disc covered, from zero to one.
	Obscuration float64
}

// obscuration corrects what Swiss Ephemeris reports for a total eclipse.
//
// For a total or annular eclipse the library gives the ratio of the two discs'
// areas. When the Moon is the smaller, in an annular eclipse, that ratio is
// exactly the fraction of the Sun it covers. When the Moon is the larger, in a
// total eclipse, the ratio passes one, and a fraction of a disc greater than
// the whole is not something to hand to a client: the Sun is wholly covered.
func obscuration(raw float64) float64 {
	if raw > 1 {
		return 1
	}
	return raw
}

// EclipseRequest asks for the eclipses in a span.
type EclipseRequest struct {
	RangeRequest

	// Kinds selects solar, lunar or both. Both is the default.
	Kinds []string `json:"kinds,omitempty"`
}

// Eclipses finds the eclipses in a span.
//
// Swiss Ephemeris has its own search for these. Solving for them here would
// mean rediscovering the geometry of the shadow cone, which the library
// already does properly, so its answer is used and only dressed for the
// response.
func (e *Engine) Eclipses(ctx context.Context, req EclipseRequest) ([]Eclipse, error) {
	from, to, err := resolveRange(req.RangeRequest)
	if err != nil {
		return nil, err
	}

	wantSolar, wantLunar := true, true
	if len(req.Kinds) > 0 {
		wantSolar, wantLunar = false, false
		for _, k := range req.Kinds {
			switch EclipseKind(k) {
			case EclipseSolar:
				wantSolar = true
			case EclipseLunar:
				wantLunar = true
			default:
				return nil, &tz.FieldError{
					Field:   "kinds",
					Message: fmt.Sprintf("%q is not an eclipse kind; use solar or lunar", k),
				}
			}
		}
	}

	var found []Eclipse

	err = e.calc.Do(ctx, func(s *swe.Session) error {
		if wantSolar {
			list, err := e.solarEclipses(s, from.JulianDayUT, to.JulianDayUT)
			if err != nil {
				return err
			}
			found = append(found, list...)
		}
		if wantLunar {
			list, err := e.lunarEclipses(s, from.JulianDayUT, to.JulianDayUT)
			if err != nil {
				return err
			}
			found = append(found, list...)
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

func (e *Engine) solarEclipses(s *swe.Session, from, to float64) ([]Eclipse, error) {
	var found []Eclipse

	search := from
	for search < to && len(found) < maxEclipses {
		raw, err := s.SolarEclipse(search, libswe.EclAllSolar, false, swe.Options{})
		if err != nil {
			return nil, err
		}

		maximum := raw.Times[0]
		if maximum > to {
			break
		}

		eclipse, err := e.describeEclipse(s, EclipseSolar, raw.Kind, maximum)
		if err != nil {
			return nil, err
		}

		// Where the shadow falls deepest, and how much of the Sun goes with
		// it. This is what makes one eclipse worth travelling for and
		// another barely noticeable.
		where, err := s.SolarEclipseWhere(maximum, swe.Options{})
		if err == nil {
			eclipse.GreatestAt = &Location{
				Latitude:  where.Latitude,
				Longitude: where.Longitude,
			}
			eclipse.Magnitude = where.Magnitude
			eclipse.Obscuration = obscuration(where.Obscuration)
		}

		found = append(found, eclipse)
		// Step past this one. Eclipses come in seasons about six months
		// apart, and never closer than a fortnight.
		search = maximum + 10
	}
	return found, nil
}

func (e *Engine) lunarEclipses(s *swe.Session, from, to float64) ([]Eclipse, error) {
	var found []Eclipse

	search := from
	for search < to && len(found) < maxEclipses {
		raw, err := s.LunarEclipse(search, libswe.EclAllLunar, false, swe.Options{})
		if err != nil {
			return nil, err
		}

		maximum := raw.Times[0]
		if maximum > to {
			break
		}

		eclipse, err := e.describeEclipse(s, EclipseLunar, raw.Kind, maximum)
		if err != nil {
			return nil, err
		}
		found = append(found, eclipse)
		search = maximum + 10
	}
	return found, nil
}

// describeEclipse fills in the shared part of an eclipse.
func (e *Engine) describeEclipse(s *swe.Session, kind EclipseKind, flags int32, jd float64) (Eclipse, error) {
	moment, err := MomentFromJD(jd)
	if err != nil {
		return Eclipse{}, err
	}

	sun, err := s.Calc(jd, libswe.Sun, swe.Options{})
	if err != nil {
		return Eclipse{}, err
	}
	moon, err := s.Calc(jd, libswe.Moon, swe.Options{})
	if err != nil {
		return Eclipse{}, err
	}

	sunPos := newPosition("sun", "Sun", sun.Longitude)
	sunPos.Latitude, sunPos.Speed = sun.Latitude, sun.SpeedLong
	moonPos := newPosition("moon", "Moon", moon.Longitude)
	moonPos.Latitude, moonPos.Speed = moon.Latitude, moon.SpeedLong

	return Eclipse{
		Kind:        kind,
		Type:        eclipseType(flags),
		Central:     flags&libswe.EclCentral != 0,
		MaximumUTC:  moment.UTC,
		JulianDayUT: jd,
		Sun:         sunPos,
		Moon:        moonPos,
	}, nil
}

// eclipseType reads the kind out of the flags Swiss Ephemeris returns.
//
// The order matters: a hybrid eclipse carries the annular bit as well, so it
// has to be recognised first or it would be reported as merely annular.
func eclipseType(flags int32) string {
	switch {
	case flags&libswe.EclAnnularTotal != 0:
		return "hybrid"
	case flags&libswe.EclTotal != 0:
		return "total"
	case flags&libswe.EclAnnular != 0:
		return "annular"
	case flags&libswe.EclPenumbral != 0:
		return "penumbral"
	case flags&libswe.EclPartial != 0:
		return "partial"
	default:
		return "unknown"
	}
}
