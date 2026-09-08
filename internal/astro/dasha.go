package astro

import (
	"context"
	"fmt"
	"time"

	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// The Vimshottari dasha divides a life into periods ruled by the nine
// Vimshottari lords, running a hundred and twenty years in all. Which lord
// opens the sequence, and how much of that first period is already spent at
// birth, are both read from where the Moon stood among the nakshatras.
//
// Each period divides again by the same sequence and in the same proportions,
// so a great period of twenty years gives its sub-period lords twenty times
// their own share of a hundred and twenty.

// YearLength is how a dasha year is measured in days.
//
// This is the single largest source of disagreement between one piece of
// software and another: the same birth data run against two conventions puts
// the period boundaries days or weeks apart. The choice is therefore explicit
// and reported back, rather than buried.
type YearLength string

const (
	// YearJulian is 365.25 days, the convention most modern software uses.
	YearJulian YearLength = "julian"

	// YearSavana is 360 days, the older reckoning of twelve thirty-day
	// months still followed by some traditions.
	YearSavana YearLength = "savana"

	// YearTropical is 365.2422 days, the year the seasons actually keep.
	YearTropical YearLength = "tropical"
)

// DefaultYearLength is the convention used when a request does not name one.
const DefaultYearLength = YearJulian

// days returns the length of a dasha year under this convention.
func (y YearLength) days() (float64, bool) {
	switch y {
	case YearJulian:
		return 365.25, true
	case YearSavana:
		return 360, true
	case YearTropical:
		return 365.2422, true
	default:
		return 0, false
	}
}

// maxDashaDepth is how far the nesting may be asked for. Three levels reach
// periods of a few days; a fourth reaches hours, and the birth time is never
// known finely enough for that to mean anything.
const maxDashaDepth = 3

// DashaPeriod is one period, with the periods it divides into.
type DashaPeriod struct {
	// Level is 1 for a mahadasha, 2 for an antardasha within it, and so on.
	Level int `json:"level"`

	// Lord is the planet ruling the period.
	Lord string `json:"lord"`

	Start time.Time `json:"start"`
	End   time.Time `json:"end"`

	// Years is the length of the period. A first period is shorter than its
	// lord's full share, because part of it was already spent at birth.
	Years float64 `json:"years"`

	// Periods are the divisions of this one, when the requested depth
	// reaches them.
	Periods []DashaPeriod `json:"periods,omitempty"`
}

// DashaRequest asks for the Vimshottari periods of a birth.
type DashaRequest struct {
	Natal ChartRequest `json:"natal"`

	// Depth is how far to nest: 1 for the great periods alone, 2 to divide
	// them, 3 to divide those again.
	Depth int `json:"depth,omitempty"`

	// YearLength selects how a dasha year is measured. See YearLength.
	YearLength string `json:"year_length,omitempty"`

	// From and To narrow the result to the periods overlapping a span. A
	// whole life at depth three is a large answer, and a client usually wants
	// the part around now.
	From *tz.Input `json:"from,omitempty"`
	To   *tz.Input `json:"to,omitempty"`
}

// DashaResult is the sequence of periods, with what it was derived from.
type DashaResult struct {
	Natal *Chart

	// Moon is where the Moon stood at birth, and Nakshatra the mansion it
	// fell in. Everything below follows from these two.
	Moon      Position
	Nakshatra NakshatraPlacement

	// StartingLord opens the sequence, and BalanceYears is how much of its
	// period remained at birth.
	StartingLord string
	BalanceYears float64

	YearLength     YearLength
	YearLengthDays float64

	Depth   int
	Periods []DashaPeriod
}

// Dashas computes the Vimshottari periods of a birth.
func (e *Engine) Dashas(ctx context.Context, req DashaRequest) (*DashaResult, error) {
	depth := req.Depth
	if depth == 0 {
		depth = 2
	}
	if depth < 1 || depth > maxDashaDepth {
		return nil, &tz.FieldError{
			Field: "depth",
			Message: fmt.Sprintf("must be between 1 and %d, got %d; below that the "+
				"periods are shorter than a birth time is ever known to", maxDashaDepth, depth),
		}
	}

	yearLength := YearLength(req.YearLength)
	if req.YearLength == "" {
		yearLength = DefaultYearLength
	}
	yearDays, ok := yearLength.days()
	if !ok {
		return nil, &tz.FieldError{
			Field: "year_length",
			Message: fmt.Sprintf("%q is not a known convention; use julian, savana or tropical",
				req.YearLength),
		}
	}

	// The nakshatras are tied to the fixed stars, so the whole scheme is
	// meaningless read off a tropical longitude. A request that does not ask
	// for sidereal gets it anyway, since asking for a dasha is asking for
	// the sidereal zodiac.
	natalReq := req.Natal
	if natalReq.Settings.Zodiac == "" {
		natalReq.Settings.Zodiac = string(ZodiacSidereal)
	}
	if natalReq.Settings.Zodiac != string(ZodiacSidereal) {
		return nil, &tz.FieldError{
			Field: "natal.settings.zodiac",
			Message: "the Vimshottari dasha is read from the nakshatras, which are fixed to " +
				"the stars, so it can only be computed in the sidereal zodiac",
		}
	}

	natal, err := e.Cast(ctx, natalReq)
	if err != nil {
		return nil, prefixField(err, "natal")
	}

	moon, err := moonOf(natal)
	if err != nil {
		return nil, err
	}

	placement := NakshatraAt(moon.Longitude)
	lordYears, ok := vimshottariYears[placement.Lord]
	if !ok {
		return nil, fmt.Errorf("astro: %s has no Vimshottari period", placement.Lord)
	}

	// The part of the first period already spent at birth is the part of the
	// nakshatra the Moon has already crossed.
	balance := lordYears * (1 - placement.Fraction)

	result := &DashaResult{
		Natal:          natal,
		Moon:           moon,
		Nakshatra:      placement,
		StartingLord:   placement.Lord,
		BalanceYears:   roundTo(balance, 6),
		YearLength:     yearLength,
		YearLengthDays: yearDays,
		Depth:          depth,
	}

	result.Periods = buildDashas(natal.Moment.UTC, placement.Lord, balance, yearDays, depth)

	if req.From != nil || req.To != nil {
		from, to, err := dashaWindow(req)
		if err != nil {
			return nil, err
		}
		result.Periods = clipPeriods(result.Periods, from, to)
	}
	return result, nil
}

// moonOf finds the Moon in a chart, which every dasha is read from.
func moonOf(chart *Chart) (Position, error) {
	for _, p := range chart.Positions {
		if p.Body == "moon" {
			return p, nil
		}
	}
	return Position{}, &tz.FieldError{
		Field:   "natal.settings.bodies",
		Message: "the Vimshottari dasha is read from the Moon, so the Moon must be among the bodies",
	}
}

// buildDashas lays out the whole sequence from the birth onwards.
//
// The first period is short by however much of it was spent before birth; every
// one after it runs its lord's full share, and the sequence cycles through the
// nine lords for as long as a life could last.
func buildDashas(birth time.Time, startingLord string, balanceYears, yearDays float64, depth int) []DashaPeriod {
	var out []DashaPeriod

	start := birth
	index := vimshottariIndex(startingLord)
	years := balanceYears

	// A hundred and twenty years is one full turn of the cycle, which is
	// longer than any life. Going once round from the first period covers it.
	for i := 0; i < len(vimshottariOrder); i++ {
		lord := vimshottariOrder[(index+i)%len(vimshottariOrder)]
		if i > 0 {
			years = vimshottariYears[lord]
		}

		end := start.Add(daysToDuration(years * yearDays))
		period := DashaPeriod{
			Level: 1,
			Lord:  lord,
			Start: start,
			End:   end,
			Years: roundTo(years, 6),
		}

		if depth > 1 {
			// A first period was entered partway through, so its sub-periods
			// start partway through too. Laying out the full period and then
			// dropping what fell before birth keeps the boundaries where they
			// belong instead of stretching the remainder to fit.
			fullStart := start
			if i == 0 {
				fullStart = end.Add(-daysToDuration(vimshottariYears[lord] * yearDays))
			}
			period.Periods = subPeriods(2, lord, fullStart, end,
				vimshottariYears[lord], yearDays, depth, start)
		}

		out = append(out, period)
		start = end
	}
	return out
}

// subPeriods divides a period among the nine lords, beginning with the lord of
// the period itself and running in the usual order. Each takes its own share of
// a hundred and twenty of the parent's length.
//
// notBefore drops the divisions that fell before birth, which only happens
// inside the first period.
func subPeriods(level int, parentLord string, parentStart, parentEnd time.Time,
	parentYears, yearDays float64, depth int, notBefore time.Time) []DashaPeriod {

	var out []DashaPeriod

	start := parentStart
	index := vimshottariIndex(parentLord)
	last := len(vimshottariOrder) - 1

	for i := 0; i < len(vimshottariOrder); i++ {
		lord := vimshottariOrder[(index+i)%len(vimshottariOrder)]
		years := parentYears * vimshottariYears[lord] / VimshottariTotalYears

		end := start.Add(daysToDuration(years * yearDays))
		if i == last {
			// The divisions tile the period they belong to exactly. Working
			// forward through nine separate conversions to a duration
			// truncates a little each time, which would leave this one ending
			// a few nanoseconds short of its parent and open a gap that has no
			// business existing.
			end = parentEnd
		}

		if !end.After(notBefore) {
			// Wholly before birth.
			start = end
			continue
		}

		period := DashaPeriod{
			Level: level,
			Lord:  lord,
			Start: start,
			End:   end,
			Years: roundTo(years, 6),
		}
		// The division that birth falls inside is entered partway through, so
		// its own start moves up while its divisions keep their places.
		if period.Start.Before(notBefore) {
			period.Start = notBefore
			period.Years = roundTo(end.Sub(notBefore).Hours()/24/yearDays, 6)
		}

		if depth > level {
			period.Periods = subPeriods(level+1, lord, start, end, years, yearDays, depth, notBefore)
		}

		out = append(out, period)
		start = end
	}
	return out
}

// daysToDuration converts a length in days to a duration, keeping the fraction.
func daysToDuration(days float64) time.Duration {
	return time.Duration(days * 24 * float64(time.Hour))
}

// dashaWindow resolves the optional span a request narrows the answer to.
func dashaWindow(req DashaRequest) (from, to time.Time, err error) {
	if req.From != nil {
		m, err := NewMoment(*req.From)
		if err != nil {
			return time.Time{}, time.Time{}, prefixField(err, "from")
		}
		from = m.UTC
	}
	if req.To != nil {
		m, err := NewMoment(*req.To)
		if err != nil {
			return time.Time{}, time.Time{}, prefixField(err, "to")
		}
		to = m.UTC
	}
	if !from.IsZero() && !to.IsZero() && !to.After(from) {
		return time.Time{}, time.Time{}, &tz.FieldError{
			Field:   "to",
			Message: "must be after from",
		}
	}
	return from, to, nil
}

// clipPeriods keeps only the periods overlapping a span, recursively.
//
// The periods themselves are not trimmed: a client asking what is running now
// wants the period with its real boundaries, not one cut off at the edge of the
// window it happened to ask about.
func clipPeriods(periods []DashaPeriod, from, to time.Time) []DashaPeriod {
	var out []DashaPeriod

	for _, p := range periods {
		if !from.IsZero() && !p.End.After(from) {
			continue
		}
		if !to.IsZero() && !p.Start.Before(to) {
			continue
		}
		if len(p.Periods) > 0 {
			p.Periods = clipPeriods(p.Periods, from, to)
		}
		out = append(out, p)
	}
	return out
}
