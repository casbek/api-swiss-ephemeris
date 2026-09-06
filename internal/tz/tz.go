// Package tz turns the date, time and zone a client sends into an exact
// instant in UTC.
//
// This is where an astrology service most often goes wrong. A birth
// certificate records a wall clock reading, and turning that into an instant
// needs the daylight saving rules that were in force on that date in that
// place, not the rules in force today. Two readings a year are also not
// instants at all: one wall clock time is skipped when the clocks go forward
// and another happens twice when they go back. Both cases are detected and
// reported rather than quietly resolved.
package tz

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	// The time zone database is embedded in the binary. Without this the
	// service depends on the host having /usr/share/zoneinfo, which a
	// minimal container image does not, and zone lookups would fail at
	// runtime on exactly the deployments that look cleanest. It costs about
	// 450 KB.
	_ "time/tzdata"
)

// unknownTimeHour is the local hour used when a client sends no time.
//
// Midday is the conventional choice: it is the point that minimises the error
// in the Moon's position across the day. Houses and angles must not be
// reported for such a chart, and TimeKnown marks it.
const unknownTimeHour = 12

// Anomaly describes a local reading that does not name exactly one instant.
type Anomaly string

const (
	// AnomalyNone means the reading names exactly one instant.
	AnomalyNone Anomaly = ""

	// AnomalyNonexistent means the clocks jumped over this reading, so it
	// never happened. The instant reported is the one the clock showed
	// after the jump.
	AnomalyNonexistent Anomaly = "nonexistent"

	// AnomalyAmbiguous means the clocks went back over this reading, so it
	// happened twice. Alternatives lists both instants.
	AnomalyAmbiguous Anomaly = "ambiguous"
)

// Source records how the offset was determined.
type Source string

const (
	// SourceIANA means a named zone supplied the historical rules.
	SourceIANA Source = "iana"
	// SourceFixedOffset means the client gave an offset directly, so no
	// daylight saving rule was applied.
	SourceFixedOffset Source = "fixed_offset"
)

// Input is the datetime object a client sends.
type Input struct {
	// Date is required, as YYYY-MM-DD.
	Date string `json:"date"`

	// Time is the local reading, as HH:MM or HH:MM:SS. It may carry a Z or
	// an offset suffix, in which case Timezone and UTCOffset must be empty.
	// When absent the chart is treated as having an unknown birth time.
	Time string `json:"time,omitempty"`

	// Timezone is an IANA name such as Europe/Istanbul. Preferred, because
	// only a named zone carries the rules that applied on a past date.
	Timezone string `json:"timezone,omitempty"`

	// UTCOffset is a fixed offset such as +03:00, taken at face value with
	// no daylight saving rule applied.
	UTCOffset string `json:"utc_offset,omitempty"`
}

// Zone describes the offset that was applied.
type Zone struct {
	Name          string `json:"name"`
	Abbreviation  string `json:"abbreviation,omitempty"`
	Offset        string `json:"offset"`
	OffsetSeconds int    `json:"offset_seconds"`
	IsDST         bool   `json:"is_dst"`
	Source        Source `json:"source"`
}

// Resolved is the outcome of turning an Input into an instant.
type Resolved struct {
	UTC   time.Time
	Local time.Time
	Zone  Zone

	// TimeKnown is false when the client sent no time and midday was
	// assumed. Houses and angles are meaningless for such a chart.
	TimeKnown bool

	// Anomaly is set when the local reading did not name exactly one
	// instant.
	Anomaly Anomaly

	// Alternatives holds every candidate instant when Anomaly is
	// AnomalyAmbiguous, earliest first. UTC is one of them.
	Alternatives []time.Time
}

// FieldError names the input field that was wrong, so the HTTP layer can point
// the client at it.
type FieldError struct {
	Field   string
	Message string
}

func (e *FieldError) Error() string { return e.Field + ": " + e.Message }

func fieldErr(field, format string, args ...any) *FieldError {
	return &FieldError{Field: field, Message: fmt.Sprintf(format, args...)}
}

// Resolve turns a client's datetime into an instant.
func Resolve(in Input) (Resolved, error) {
	y, mo, d, err := parseDate(in.Date)
	if err != nil {
		return Resolved{}, err
	}

	clock, err := parseTime(in.Time)
	if err != nil {
		return Resolved{}, err
	}

	// The offset may come from the time string, from a named zone or from an
	// explicit offset, and naming more than one is a contradiction rather
	// than something to resolve by precedence.
	if clock.hasOffset && (in.Timezone != "" || in.UTCOffset != "") {
		return Resolved{}, fieldErr("datetime.time",
			"carries its own offset, so timezone and utc_offset must be omitted")
	}
	if in.Timezone != "" && in.UTCOffset != "" {
		return Resolved{}, fieldErr("datetime.timezone",
			"cannot be combined with utc_offset; give one or the other")
	}
	if !clock.hasOffset && in.Timezone == "" && in.UTCOffset == "" {
		return Resolved{}, fieldErr("datetime.timezone",
			"required: give an IANA zone name such as Europe/Istanbul, a utc_offset, or a time ending in Z or an offset")
	}

	switch {
	case clock.hasOffset:
		return resolveFixed(y, mo, d, clock, clock.offsetSeconds, "", SourceFixedOffset), nil
	case in.UTCOffset != "":
		off, err := parseOffset(in.UTCOffset, "datetime.utc_offset")
		if err != nil {
			return Resolved{}, err
		}
		return resolveFixed(y, mo, d, clock, off, "", SourceFixedOffset), nil
	default:
		return resolveNamed(y, mo, d, clock, in.Timezone)
	}
}

// resolveFixed applies an offset directly. No daylight saving rule exists for
// a bare offset, so no anomaly is possible.
func resolveFixed(y int, mo time.Month, d int, c clock, offsetSeconds int, name string, src Source) Resolved {
	if name == "" {
		name = formatOffset(offsetSeconds)
		if offsetSeconds == 0 {
			name = "UTC"
		}
	}
	loc := time.FixedZone(name, offsetSeconds)
	local := time.Date(y, mo, d, c.hour, c.minute, c.second, 0, loc)

	return Resolved{
		UTC:       local.UTC(),
		Local:     local,
		TimeKnown: c.known,
		Zone: Zone{
			Name:          name,
			Offset:        formatOffset(offsetSeconds),
			OffsetSeconds: offsetSeconds,
			Source:        src,
		},
	}
}

// resolveNamed applies the historical rules of an IANA zone.
func resolveNamed(y int, mo time.Month, d int, c clock, zone string) (Resolved, error) {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return Resolved{}, fieldErr("datetime.timezone",
			"%q is not an IANA time zone name; use a name such as Europe/Istanbul", zone)
	}
	// LoadLocation accepts "Local", which would resolve against whatever the
	// server happens to be set to and make results depend on the host.
	if strings.EqualFold(zone, "Local") {
		return Resolved{}, fieldErr("datetime.timezone",
			"\"Local\" is not accepted; name the zone explicitly, such as Europe/Istanbul")
	}

	local, anomaly, alternatives := classify(y, mo, d, c, loc)
	abbrev, offset := local.Zone()

	return Resolved{
		UTC:          local.UTC(),
		Local:        local,
		TimeKnown:    c.known,
		Anomaly:      anomaly,
		Alternatives: alternatives,
		Zone: Zone{
			Name:          zone,
			Abbreviation:  abbrev,
			Offset:        formatOffset(offset),
			OffsetSeconds: offset,
			IsDST:         local.IsDST(),
			Source:        SourceIANA,
		},
	}, nil
}

// classify builds the instant and reports whether the reading was skipped or
// repeated by a clock change.
//
// Go resolves both cases silently: a skipped reading comes back shifted past
// the gap, and a repeated one comes back as the later of the two. Detecting
// them takes comparing what was asked for with what came back.
func classify(y int, mo time.Month, d int, c clock, loc *time.Location) (time.Time, Anomaly, []time.Time) {
	t := time.Date(y, mo, d, c.hour, c.minute, c.second, 0, loc)

	// A reading the clocks jumped over comes back as a different wall clock
	// reading, because there is no instant that shows it.
	if t.Year() != y || t.Month() != mo || t.Day() != d ||
		t.Hour() != c.hour || t.Minute() != c.minute || t.Second() != c.second {
		return t, AnomalyNonexistent, nil
	}

	// A repeated reading has a second instant, one clock change away, that
	// shows the same wall clock with a different offset. Transitions are at
	// most two hours in practice, and checking both directions covers a
	// reading that landed on either side.
	_, offset := t.Zone()
	for _, delta := range []time.Duration{-2 * time.Hour, -time.Hour, time.Hour, 2 * time.Hour} {
		alt := t.Add(delta)
		if alt.Year() != y || alt.Month() != mo || alt.Day() != d ||
			alt.Hour() != c.hour || alt.Minute() != c.minute || alt.Second() != c.second {
			continue
		}
		if _, altOffset := alt.Zone(); altOffset != offset {
			candidates := []time.Time{t.UTC(), alt.UTC()}
			if candidates[1].Before(candidates[0]) {
				candidates[0], candidates[1] = candidates[1], candidates[0]
			}
			return t, AnomalyAmbiguous, candidates
		}
	}
	return t, AnomalyNone, nil
}

// clock is a parsed time of day.
type clock struct {
	hour, minute, second int

	// known is false when the client sent no time and midday was assumed.
	known bool

	hasOffset     bool
	offsetSeconds int
}

func parseDate(s string) (int, time.Month, int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, 0, fieldErr("datetime.date", "required, as YYYY-MM-DD")
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return 0, 0, 0, fieldErr("datetime.date",
			"%q is not a valid date; use YYYY-MM-DD", s)
	}
	return t.Year(), t.Month(), t.Day(), nil
}

// parseTime reads a time of day, with an optional Z or offset suffix.
func parseTime(s string) (clock, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		// No time given: the birth time is unknown.
		return clock{hour: unknownTimeHour, known: false}, nil
	}

	c := clock{known: true}

	// Split off a trailing zone designator before reading the clock, so the
	// two are validated separately and the error names the right part.
	body := s
	switch {
	case strings.HasSuffix(body, "Z"), strings.HasSuffix(body, "z"):
		body = body[:len(body)-1]
		c.hasOffset = true
		c.offsetSeconds = 0
	default:
		// An offset starts at a sign that comes after the hour, so only
		// look past the first character.
		if i := strings.IndexAny(body[1:], "+-"); i >= 0 {
			i++
			off, err := parseOffset(body[i:], "datetime.time")
			if err != nil {
				return clock{}, err
			}
			body = body[:i]
			c.hasOffset = true
			c.offsetSeconds = off
		}
	}

	var t time.Time
	var err error
	for _, layout := range []string{"15:04:05", "15:04"} {
		if t, err = time.Parse(layout, body); err == nil {
			break
		}
	}
	if err != nil {
		return clock{}, fieldErr("datetime.time",
			"%q is not a valid time; use HH:MM or HH:MM:SS", s)
	}

	c.hour, c.minute, c.second = t.Hour(), t.Minute(), t.Second()
	return c, nil
}

// parseOffset reads an offset such as +03:00, -0530, +03 or Z.
func parseOffset(s, field string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fieldErr(field, "empty offset")
	}
	if strings.EqualFold(s, "Z") || strings.EqualFold(s, "UTC") {
		return 0, nil
	}

	sign := 1
	switch s[0] {
	case '+':
		s = s[1:]
	case '-':
		sign = -1
		s = s[1:]
	default:
		return 0, fieldErr(field, "%q must start with + or -, as in +03:00", s)
	}

	var hh, mm string
	switch {
	case strings.Contains(s, ":"):
		hh, mm, _ = strings.Cut(s, ":")
	case len(s) == 4:
		hh, mm = s[:2], s[2:]
	case len(s) <= 2:
		hh, mm = s, "0"
	default:
		return 0, fieldErr(field, "%q is not a valid offset; use +HH:MM", s)
	}

	h, err := strconv.Atoi(hh)
	if err != nil {
		return 0, fieldErr(field, "%q is not a valid offset; use +HH:MM", s)
	}
	m, err := strconv.Atoi(mm)
	if err != nil {
		return 0, fieldErr(field, "%q is not a valid offset; use +HH:MM", s)
	}
	// Real offsets run from -12:00 to +14:00. Anything wider is a mistake,
	// and accepting it would silently produce a chart for the wrong day.
	if h > 14 || m < 0 || m > 59 {
		return 0, fieldErr(field, "%q is outside the range of real time zone offsets", s)
	}

	total := sign * (h*3600 + m*60)
	if total < -12*3600 || total > 14*3600 {
		return 0, fieldErr(field, "%q is outside the range of real time zone offsets", s)
	}
	return total, nil
}

// formatOffset renders an offset in seconds as +HH:MM.
func formatOffset(seconds int) string {
	sign := "+"
	if seconds < 0 {
		sign = "-"
		seconds = -seconds
	}
	return fmt.Sprintf("%s%02d:%02d", sign, seconds/3600, (seconds%3600)/60)
}
