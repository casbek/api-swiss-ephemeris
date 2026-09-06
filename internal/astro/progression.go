package astro

import (
	"context"
	"fmt"

	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// tropicalYear is the length of the year the seasons follow, in days. It is
// the measure secondary progressions use: one day of the ephemeris stands for
// one year of life, and this is how long that year is.
const tropicalYear = 365.242190

// ProgressionMethod selects how a chart is moved forward.
type ProgressionMethod string

const (
	// ProgressionSecondary counts a day of the ephemeris for a year of life.
	// The progressed chart is a real chart, cast for a moment shortly after
	// the birth, and every body moves at its own rate.
	ProgressionSecondary ProgressionMethod = "secondary"

	// ProgressionSolarArc moves every point forward by the same arc: the
	// distance the progressed Sun has travelled. The whole chart keeps its
	// shape, so aspects within it never change.
	ProgressionSolarArc ProgressionMethod = "solar_arc"
)

// DefaultProgressionMethod is used when a request does not name one.
const DefaultProgressionMethod = ProgressionSecondary

// ProgressionRequest asks for a birth chart moved forward to a later date.
type ProgressionRequest struct {
	Natal ChartRequest `json:"natal"`

	// Target is the date to progress to.
	Target MomentRequest `json:"target"`

	Method   string         `json:"method,omitempty"`
	Settings *SettingsInput `json:"settings,omitempty"`
}

// ProgressionResult is a birth chart and its progression.
type ProgressionResult struct {
	Method ProgressionMethod
	Natal  *Chart

	// Target is the date progressed to, and ElapsedYears the age at it.
	Target       Moment
	ElapsedYears float64

	// ProgressedMoment is the moment the secondary chart was cast for: a few
	// weeks after the birth for a life of ordinary length. It is empty for a
	// solar arc, which is not cast for a moment at all.
	ProgressedMoment *Moment

	// Arc is the distance every point was moved, for a solar arc.
	Arc float64

	Positions []Position
	Houses    *Houses

	// Aspects run from a progressed point to a natal one.
	Aspects []Aspect

	Settings Settings
	Notes    []string
}

// Progress moves a birth chart forward to a later date.
func (e *Engine) Progress(ctx context.Context, req ProgressionRequest) (*ProgressionResult, error) {
	method := ProgressionMethod(req.Method)
	if req.Method == "" {
		method = DefaultProgressionMethod
	}
	if method != ProgressionSecondary && method != ProgressionSolarArc {
		return nil, &tz.FieldError{
			Field: "method",
			Message: fmt.Sprintf("%q is not a progression method; use secondary or solar_arc",
				req.Method),
		}
	}

	natal, err := e.Cast(ctx, req.Natal)
	if err != nil {
		return nil, prefixField(err, "natal")
	}

	target, err := NewMoment(req.Target.DateTime)
	if err != nil {
		return nil, prefixField(err, "target")
	}

	settings := natal.Settings
	if req.Settings != nil {
		if settings, err = ResolveSettings(*req.Settings, req.Natal.Location); err != nil {
			return nil, err
		}
	}

	elapsed := (target.JulianDayUT - natal.Moment.JulianDayUT) / tropicalYear

	// The progressed moment is the birth plus one day for each elapsed year.
	// A life of eighty years reaches eighty days past the birth, so this
	// stays well inside the ephemeris whatever the target date.
	progressedJD := natal.Moment.JulianDayUT + elapsed
	progressedMoment, err := MomentFromJD(progressedJD)
	if err != nil {
		return nil, err
	}

	// The progressed chart is cast for the birthplace. The angles then move
	// at the rate the sky turned in the days after the birth, which is what
	// secondary progression means by a progressed ascendant.
	progressed, err := e.cast(ctx, progressedMoment, req.Natal.Location, settings)
	if err != nil {
		return nil, err
	}

	result := &ProgressionResult{
		Method:       method,
		Natal:        natal,
		Target:       target,
		ElapsedYears: elapsed,
		Settings:     settings,
	}

	if elapsed < 0 {
		result.Notes = append(result.Notes,
			"The target date is before the birth, so the chart has been progressed "+
				"backwards. That is a valid calculation but an unusual request.")
	}

	switch method {
	case ProgressionSecondary:
		result.ProgressedMoment = &progressed.Moment
		result.Positions = progressed.Positions
		result.Houses = progressed.Houses
		result.Notes = append(result.Notes,
			"The houses here belong to the progressed chart itself. Reading "+
				"progressed bodies against the natal houses instead is equally "+
				"common; the natal houses are in the natal half of this response.")

	case ProgressionSolarArc:
		arc, err := solarArc(natal, progressed)
		if err != nil {
			return nil, err
		}
		result.Arc = arc
		result.Positions, result.Houses = directBy(natal, arc)
		result.Notes = append(result.Notes,
			"Every point was moved by the same arc, so the aspects within the "+
				"directed chart are identical to the natal ones. Only the contacts "+
				"with the natal chart carry information.")
	}

	// Houses, sect and dignity, in that order, as for any chart.
	sect := natal.Sect
	if result.Houses != nil {
		for i := range result.Positions {
			result.Houses.assign(&result.Positions[i])
		}
		if method == ProgressionSecondary {
			sect = SectOf(sunHouse(result.Positions))
		}
	}
	for i := range result.Positions {
		p := &result.Positions[i]
		p.Dignity = DignityOf(p.Body, p.Longitude, sect)
	}

	progressedPoints := result.Positions
	if result.Houses != nil {
		progressedPoints = append(append([]Position(nil), result.Positions...),
			result.Houses.Angles...)
	}
	result.Aspects = CrossAspects(progressedPoints, natalTargets(natal),
		settings.AspectTypes, settings.Orbs)

	return result, nil
}

// solarArc is the distance the Sun has travelled from its natal place to its
// progressed one.
//
// The Sun only ever moves forward, so the arc is measured the long way if need
// be: someone of seventy has a solar arc of about seventy degrees, and the
// shorter way round would be the wrong answer past a hundred and eighty.
func solarArc(natal, progressed *Chart) (float64, error) {
	var natalSun, progressedSun float64
	var foundNatal, foundProgressed bool

	for _, p := range natal.Positions {
		if p.Body == "sun" {
			natalSun, foundNatal = p.Longitude, true
		}
	}
	for _, p := range progressed.Positions {
		if p.Body == "sun" {
			progressedSun, foundProgressed = p.Longitude, true
		}
	}
	if !foundNatal || !foundProgressed {
		return 0, &tz.FieldError{
			Field:   "settings.bodies",
			Message: "a solar arc is measured from the Sun, so the Sun must be among the bodies",
		}
	}
	return Normalize(progressedSun - natalSun), nil
}

// directBy moves every natal point forward by the same arc.
func directBy(natal *Chart, arc float64) ([]Position, *Houses) {
	positions := make([]Position, 0, len(natal.Positions))
	for _, p := range natal.Positions {
		directed := newPosition(p.Body, p.Label, p.Longitude+arc)
		directed.Latitude = p.Latitude
		directed.DistanceAU = p.DistanceAU
		directed.Declination = p.Declination
		// A directed point carries no motion of its own: the whole chart was
		// moved as one, so nothing is faster or slower than anything else.
		directed.Speed = 0
		positions = append(positions, directed)
	}

	if natal.Houses == nil {
		return positions, nil
	}

	houses := &Houses{
		System:      natal.Houses.System,
		SystemLabel: natal.Houses.SystemLabel,
		ARMC:        natal.Houses.ARMC,
		Cusps:       make([]Cusp, 0, len(natal.Houses.Cusps)),
		Angles:      make([]Position, 0, len(natal.Houses.Angles)),
	}
	for _, c := range natal.Houses.Cusps {
		lon := Normalize(c.Longitude + arc)
		sign := SignAt(lon)
		houses.Cusps = append(houses.Cusps, Cusp{
			House:        c.House,
			Longitude:    lon,
			Sign:         sign.Name,
			SignIndex:    sign.Index,
			DegreeInSign: DegreeInSign(lon),
			Formatted:    FormatPosition(lon),
		})
	}
	for _, a := range natal.Houses.Angles {
		houses.Angles = append(houses.Angles,
			newPosition(a.Body, a.Label, a.Longitude+arc))
	}
	return positions, houses
}
