package astro

import (
	"fmt"
	"time"

	"github.com/casbek/api-swiss-ephemeris/internal/swe"
	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// The ephemeris files shipped with the service cover these years. The range
// was measured rather than read off the file names: outside it Swiss Ephemeris
// does not fail, it falls back to the Moshier ephemeris and keeps answering
// with less precision and only a warning. Refusing the request is better than
// returning a quietly worse chart.
//
// Adding sepl_12.se1 and its companions would extend the range backwards, and
// sepl_24.se1 forwards.
const (
	MinYear = 1800
	MaxYear = 2399
)

// Moment is an instant, expressed every way the rest of the service needs it.
type Moment struct {
	UTC   time.Time
	Local time.Time
	Zone  tz.Zone

	// TimeKnown is false when no birth time was given and midday was
	// assumed. Houses and angles must not be reported for such a chart.
	TimeKnown bool

	// Anomaly and Alternatives describe a local reading that the clocks
	// skipped or repeated. See package tz.
	Anomaly      tz.Anomaly
	Alternatives []time.Time

	// JulianDayUT is the value Swiss Ephemeris takes for position
	// calculations. JulianDayTT is the same instant in Terrestrial Time.
	JulianDayUT float64
	JulianDayTT float64
}

// NewMoment resolves a client's datetime and converts it to a Julian Day.
func NewMoment(in tz.Input) (Moment, error) {
	resolved, err := tz.Resolve(in)
	if err != nil {
		return Moment{}, err
	}

	utc := resolved.UTC
	if y := utc.Year(); y < MinYear || y > MaxYear {
		return Moment{}, &tz.FieldError{
			Field: "datetime.date",
			Message: fmt.Sprintf(
				"the year %d is outside the range this service covers, %d to %d",
				y, MinYear, MaxYear),
		}
	}

	// swe_utc_to_jd is used rather than a plain calendar conversion because
	// it accounts for leap seconds, which a naive conversion ignores.
	tt, ut, err := swe.UTCToJD(
		utc.Year(), int(utc.Month()), utc.Day(),
		utc.Hour(), utc.Minute(), float64(utc.Second())+float64(utc.Nanosecond())/1e9,
	)
	if err != nil {
		return Moment{}, err
	}

	return Moment{
		UTC:          utc,
		Local:        resolved.Local,
		Zone:         resolved.Zone,
		TimeKnown:    resolved.TimeKnown,
		Anomaly:      resolved.Anomaly,
		Alternatives: resolved.Alternatives,
		JulianDayUT:  ut,
		JulianDayTT:  tt,
	}, nil
}
