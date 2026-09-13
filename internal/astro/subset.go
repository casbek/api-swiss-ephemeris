package astro

import (
	"context"
	"fmt"

	"github.com/casbek/api-swiss-ephemeris/internal/libswe"
	"github.com/casbek/api-swiss-ephemeris/internal/swe"
	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// A natal chart answers several questions at once: where the bodies are, how
// the sky is divided where you stand, and how the two relate. A client that
// wants only one of those should not have to ask for all three and discard the
// rest, so each is available on its own.

// PositionsRequest asks where the bodies are at a moment.
type PositionsRequest struct {
	DateTime tz.Input `json:"datetime"`

	// Location is optional. Where the bodies are is the same question
	// wherever it is asked from, so it is only needed for topocentric
	// positions, which are measured from the observer rather than from the
	// centre of the Earth.
	Location *Location `json:"location,omitempty"`

	Settings SettingsInput `json:"settings,omitempty"`
}

// PositionsResult is where the bodies stood.
type PositionsResult struct {
	Moment   Moment
	Location Location
	Settings Settings

	Positions []Position

	// Obliquity is the tilt of the Earth's axis at this moment, which the
	// declinations are derived from.
	Obliquity float64

	// Ayanamsha is the offset applied, in degrees. Only set for a sidereal
	// request.
	Ayanamsha float64
}

// Positions computes where the requested bodies are, and nothing else.
//
// There are no houses here and no aspects, so there is also no essential
// dignity: the sect a chart is read by depends on which side of the horizon
// the Sun is, and there is no horizon without a place and a time of day. Ask
// for a natal chart when you want those.
func (e *Engine) Positions(ctx context.Context, req PositionsRequest) (*PositionsResult, error) {
	location := Location{}
	if req.Location != nil {
		location = *req.Location
	}

	moment, settings, err := e.prepare(req.DateTime, location, req.Settings)
	if err != nil {
		return nil, err
	}

	// A topocentric position is measured from somewhere. Left to default, the
	// somewhere would be the point off the coast of Africa where the equator
	// meets the prime meridian, and the answer would be quietly wrong rather
	// than obviously so.
	if settings.Topocentric && req.Location == nil {
		return nil, &tz.FieldError{
			Field: "location",
			Message: "required when topocentric positions are asked for, since they are " +
				"measured from the observer rather than from the centre of the Earth",
		}
	}

	result := &PositionsResult{
		Moment:   moment,
		Location: location,
		Settings: settings,
	}

	opts := settings.sweOptions(location)
	jd := moment.JulianDayUT

	err = e.calc.Do(ctx, func(s *swe.Session) error {
		eclNut, err := s.Calc(jd, libswe.EclNut, swe.Options{})
		if err != nil {
			return fmt.Errorf("obliquity: %w", err)
		}
		result.Obliquity = eclNut.Longitude

		if settings.Zodiac == ZodiacSidereal {
			if result.Ayanamsha, err = s.Ayanamsha(jd, opts); err != nil {
				return fmt.Errorf("ayanamsha: %w", err)
			}
		}

		result.Positions, err = e.positions(s, jd, settings, opts, result.Obliquity)
		return err
	})
	if err != nil {
		return nil, err
	}

	addNakshatras(settings, result.Positions)
	return result, nil
}

// HousesRequest asks how the sky is divided at a place and a moment.
type HousesRequest struct {
	DateTime tz.Input `json:"datetime"`

	// Location is required. A house division is a division of the sky as seen
	// from somewhere, and every cusp in it moves with the place.
	Location *Location `json:"location"`

	Settings SettingsInput `json:"settings,omitempty"`
}

// HousesResult is the division of the sky, with what it was derived from.
type HousesResult struct {
	Moment   Moment
	Location Location
	Settings Settings

	Houses *Houses

	Obliquity float64
	Ayanamsha float64
}

// Houses computes the house cusps and the angles, and nothing else.
//
// No bodies are calculated, so nothing is placed in the houses; the cusps are
// the answer. A natal chart is the endpoint that puts the two together.
func (e *Engine) Houses(ctx context.Context, req HousesRequest) (*HousesResult, error) {
	// Unlike the bodies, the division depends entirely on where you stand.
	// Defaulting it would answer a question nobody asked, for a place in the
	// Gulf of Guinea.
	if req.Location == nil {
		return nil, &tz.FieldError{
			Field: "location",
			Message: "required: a house division is a division of the sky as seen from " +
				"somewhere, and every cusp moves with the place",
		}
	}

	moment, settings, err := e.prepare(req.DateTime, *req.Location, req.Settings)
	if err != nil {
		return nil, err
	}

	// The sky turns a full circle a day, so the division is set by the time of
	// day more than by anything else. Without it there is nothing to divide,
	// and midday would be a guess presented as an answer.
	if !moment.TimeKnown {
		return nil, &tz.FieldError{
			Field: "datetime.time",
			Message: "required: the houses turn with the sky, a full circle a day, so " +
				"without the time of day there is nothing to divide",
		}
	}

	result := &HousesResult{
		Moment:   moment,
		Location: *req.Location,
		Settings: settings,
	}

	opts := settings.sweOptions(*req.Location)
	jd := moment.JulianDayUT

	err = e.calc.Do(ctx, func(s *swe.Session) error {
		eclNut, err := s.Calc(jd, libswe.EclNut, swe.Options{})
		if err != nil {
			return fmt.Errorf("obliquity: %w", err)
		}
		result.Obliquity = eclNut.Longitude

		if settings.Zodiac == ZodiacSidereal {
			if result.Ayanamsha, err = s.Ayanamsha(jd, opts); err != nil {
				return fmt.Errorf("ayanamsha: %w", err)
			}
		}

		raw, err := s.Houses(jd, req.Location.Latitude, req.Location.Longitude,
			settings.HouseSystem.Letter, opts)
		if err != nil {
			return fmt.Errorf("houses: %w", err)
		}
		result.Houses = buildHouses(settings.HouseSystem, raw.Cusps, raw.ASCMC)
		return nil
	})
	if err != nil {
		return nil, err
	}

	addNakshatras(settings, result.Houses.Angles)
	return result, nil
}
