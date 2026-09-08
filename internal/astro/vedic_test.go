package astro

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// The nakshatra, dasha and varga tables are transcribed from tradition, so as
// with the dignities they are checked against the structural rules the schemes
// obey rather than trusted as typed.

// TestNakshatrasTileTheCircle checks that the twenty seven mansions cover the
// zodiac exactly, with no gap and no overlap.
func TestNakshatrasTileTheCircle(t *testing.T) {
	list := Nakshatras()
	if len(list) != 27 {
		t.Fatalf("got %d nakshatras, want 27", len(list))
	}

	seen := map[string]bool{}
	for i, n := range list {
		if n.Number != i+1 {
			t.Errorf("%s is numbered %d, want %d", n.Name, n.Number, i+1)
		}
		if seen[n.Name] {
			t.Errorf("duplicate nakshatra %q", n.Name)
		}
		seen[n.Name] = true

		want := float64(i) * NakshatraSpan
		if math.Abs(n.StartLongitude-want) > 1e-9 {
			t.Errorf("%s starts at %.6f, want %.6f", n.Name, n.StartLongitude, want)
		}
	}

	// Twenty seven spans of 13°20' make exactly one circle.
	if total := float64(len(list)) * NakshatraSpan; math.Abs(total-360) > 1e-9 {
		t.Errorf("the mansions cover %.6f degrees, want 360", total)
	}
	// Four padas make one mansion.
	if math.Abs(PadaSpan*PadasPerNakshatra-NakshatraSpan) > 1e-9 {
		t.Error("the padas do not make up a whole nakshatra")
	}
}

// TestNakshatraLordsRepeatThreeTimes checks the sequence of rulers. Twenty
// seven mansions over nine lords means the cycle turns exactly three times, and
// a mistyped name anywhere would break that.
func TestNakshatraLordsRepeatThreeTimes(t *testing.T) {
	list := Nakshatras()

	for i, n := range list {
		want := vimshottariOrder[i%9]
		if n.Lord != want {
			t.Errorf("%s is ruled by %s, want %s", n.Name, n.Lord, want)
		}
	}

	// Each lord takes exactly three mansions.
	count := map[string]int{}
	for _, n := range list {
		count[n.Lord]++
	}
	for _, lord := range vimshottariOrder {
		if count[lord] != 3 {
			t.Errorf("%s rules %d mansions, want 3", lord, count[lord])
		}
	}

	// The first mansion is Ketu's, which is where the whole scheme starts.
	if list[0].Name != "ashwini" || list[0].Lord != "ketu" {
		t.Errorf("the first mansion is %s ruled by %s, want ashwini ruled by ketu",
			list[0].Name, list[0].Lord)
	}
}

// TestVimshottariYearsSumToTheCycle checks the period lengths.
func TestVimshottariYearsSumToTheCycle(t *testing.T) {
	total := 0.0
	for _, lord := range vimshottariOrder {
		years, ok := vimshottariYears[lord]
		if !ok {
			t.Errorf("%s has no period length", lord)
			continue
		}
		if years <= 0 {
			t.Errorf("%s has a period of %g years", lord, years)
		}
		total += years
	}
	if total != VimshottariTotalYears {
		t.Errorf("the periods sum to %g years, want %g", total, VimshottariTotalYears)
	}
	if len(vimshottariYears) != len(vimshottariOrder) {
		t.Errorf("there are %d period lengths for %d lords",
			len(vimshottariYears), len(vimshottariOrder))
	}
}

// TestNakshatraAtBoundaries checks the placement at the edges of a mansion and
// of its quarters, which is where an off-by-one would show.
func TestNakshatraAtBoundaries(t *testing.T) {
	cases := []struct {
		longitude float64
		number    int
		pada      int
	}{
		{0, 1, 1},                      // the very start of Ashwini
		{3.3333, 1, 1},                 // just inside the first pada
		{PadaSpan, 1, 2},               // the start of the second pada
		{NakshatraSpan - 0.0001, 1, 4}, // the last moment of Ashwini
		{NakshatraSpan, 2, 1},          // the start of Bharani
		{359.9999, 27, 4},              // the last moment of Revati
		{360, 1, 1},                    // wraps to the start again
	}

	for _, tc := range cases {
		got := NakshatraAt(tc.longitude)
		if got.Number != tc.number {
			t.Errorf("NakshatraAt(%g) is number %d, want %d", tc.longitude, got.Number, tc.number)
		}
		if got.Pada != tc.pada {
			t.Errorf("NakshatraAt(%g) is pada %d, want %d", tc.longitude, got.Pada, tc.pada)
		}
		if got.Fraction < 0 || got.Fraction >= 1 {
			t.Errorf("NakshatraAt(%g) has fraction %g, outside [0, 1)", tc.longitude, got.Fraction)
		}
	}

	// Every degree of the circle must land in some mansion and some quarter.
	for lon := 0.0; lon < 360; lon += 0.37 {
		p := NakshatraAt(lon)
		if p.Number < 1 || p.Number > 27 {
			t.Fatalf("NakshatraAt(%g) is number %d", lon, p.Number)
		}
		if p.Pada < 1 || p.Pada > 4 {
			t.Fatalf("NakshatraAt(%g) is pada %d", lon, p.Pada)
		}
	}
}

// TestSiderealChartCarriesNakshatras checks that the mansions are reported
// where they mean something and withheld where they do not.
func TestSiderealChartCarriesNakshatras(t *testing.T) {
	tropical := castReference(t, nil)
	for _, p := range tropical.Positions {
		if p.Nakshatra != nil {
			t.Errorf("%s carries a nakshatra in a tropical chart, where reading one "+
				"off the longitude would be about 24 degrees out", p.Body)
		}
	}

	sidereal := castReference(t, func(r *ChartRequest) { r.Settings.Zodiac = "sidereal" })
	for _, p := range sidereal.Positions {
		if p.Nakshatra == nil {
			t.Errorf("%s carries no nakshatra in a sidereal chart", p.Body)
			continue
		}
		// The placement must agree with the longitude it came from.
		want := NakshatraAt(p.Longitude)
		if p.Nakshatra.Number != want.Number || p.Nakshatra.Pada != want.Pada {
			t.Errorf("%s is in nakshatra %d pada %d, but its longitude %.4f gives %d pada %d",
				p.Body, p.Nakshatra.Number, p.Nakshatra.Pada,
				p.Longitude, want.Number, want.Pada)
		}
	}
	if sidereal.Houses != nil {
		for _, a := range sidereal.Houses.Angles {
			if a.Nakshatra == nil {
				t.Errorf("the %s carries no nakshatra in a sidereal chart", a.Body)
			}
		}
	}
}

func dashaRequest(adjust func(*DashaRequest)) DashaRequest {
	req := DashaRequest{Natal: referenceRequest}
	if adjust != nil {
		adjust(&req)
	}
	return req
}

// TestDashaSequenceIsWellFormed checks the shape of the whole sequence.
func TestDashaSequenceIsWellFormed(t *testing.T) {
	e := newTestEngine(t)

	result, err := e.Dashas(context.Background(), dashaRequest(func(r *DashaRequest) {
		r.Depth = 1
	}))
	if err != nil {
		t.Fatalf("Dashas failed: %v", err)
	}

	if len(result.Periods) != 9 {
		t.Fatalf("got %d great periods, want one per lord", len(result.Periods))
	}

	// The sequence opens with the lord of the Moon's mansion.
	if result.StartingLord != result.Nakshatra.Lord {
		t.Errorf("the sequence opens with %s but the Moon is in a mansion of %s",
			result.StartingLord, result.Nakshatra.Lord)
	}
	if result.Periods[0].Lord != result.StartingLord {
		t.Errorf("the first period is %s, want %s", result.Periods[0].Lord, result.StartingLord)
	}

	// The first period begins at birth, and each one begins where the last
	// ended, with no gap.
	if !result.Periods[0].Start.Equal(result.Natal.Moment.UTC) {
		t.Errorf("the first period starts at %v, want the birth at %v",
			result.Periods[0].Start, result.Natal.Moment.UTC)
	}
	for i := range result.Periods {
		p := result.Periods[i]
		if !p.End.After(p.Start) {
			t.Errorf("period %d (%s) ends before it begins", i, p.Lord)
		}
		if i > 0 && !p.Start.Equal(result.Periods[i-1].End) {
			t.Errorf("period %d (%s) does not begin where period %d ended", i, p.Lord, i-1)
		}
		// The lords run in the fixed order, cycling.
		want := vimshottariOrder[(vimshottariIndex(result.StartingLord)+i)%9]
		if p.Lord != want {
			t.Errorf("period %d is %s, want %s", i, p.Lord, want)
		}
	}

	// Only the first period is short. Every other runs its full share.
	for i := 1; i < len(result.Periods); i++ {
		p := result.Periods[i]
		if math.Abs(p.Years-vimshottariYears[p.Lord]) > 1e-6 {
			t.Errorf("the %s period runs %g years, want its full %g",
				p.Lord, p.Years, vimshottariYears[p.Lord])
		}
	}
	full := vimshottariYears[result.StartingLord]
	if result.BalanceYears <= 0 || result.BalanceYears > full {
		t.Errorf("the balance is %g years of a %g year period", result.BalanceYears, full)
	}
	if math.Abs(result.Periods[0].Years-result.BalanceYears) > 1e-6 {
		t.Errorf("the first period runs %g years but the balance is %g",
			result.Periods[0].Years, result.BalanceYears)
	}

	// The whole run covers the cycle less what was spent before birth.
	total := 0.0
	for _, p := range result.Periods {
		total += p.Years
	}
	spent := full - result.BalanceYears
	if math.Abs(total-(VimshottariTotalYears-spent)) > 1e-5 {
		t.Errorf("the sequence covers %g years, want %g",
			total, VimshottariTotalYears-spent)
	}
}

// TestDashaBalanceFollowsTheMoon checks the one number everything else hangs
// on: how much of the first period was already spent at birth is the part of
// the mansion the Moon had already crossed.
func TestDashaBalanceFollowsTheMoon(t *testing.T) {
	e := newTestEngine(t)

	result, err := e.Dashas(context.Background(), dashaRequest(nil))
	if err != nil {
		t.Fatalf("Dashas failed: %v", err)
	}

	full := vimshottariYears[result.Nakshatra.Lord]
	want := full * (1 - result.Nakshatra.Fraction)
	if math.Abs(result.BalanceYears-want) > 1e-5 {
		t.Errorf("the balance is %g years, but the Moon is %.4f of the way through "+
			"a mansion of %s, which leaves %g",
			result.BalanceYears, result.Nakshatra.Fraction, result.Nakshatra.Lord, want)
	}
}

// TestDashaSubPeriodsTileTheirParent checks the nesting. Each period must be
// divided among the nine lords in proportion, beginning with its own lord, and
// the divisions must cover it exactly.
func TestDashaSubPeriodsTileTheirParent(t *testing.T) {
	e := newTestEngine(t)

	result, err := e.Dashas(context.Background(), dashaRequest(func(r *DashaRequest) {
		r.Depth = 2
	}))
	if err != nil {
		t.Fatalf("Dashas failed: %v", err)
	}

	// The first period was entered partway through, so some of its divisions
	// fall before birth and are left out. Every later one is whole.
	for i := 1; i < len(result.Periods); i++ {
		parent := result.Periods[i]

		if len(parent.Periods) != 9 {
			t.Errorf("the %s period has %d divisions, want one per lord",
				parent.Lord, len(parent.Periods))
			continue
		}
		if parent.Periods[0].Lord != parent.Lord {
			t.Errorf("the %s period opens with a division of %s, want its own lord",
				parent.Lord, parent.Periods[0].Lord)
		}
		if !parent.Periods[0].Start.Equal(parent.Start) {
			t.Errorf("the divisions of the %s period do not begin with it", parent.Lord)
		}
		last := parent.Periods[len(parent.Periods)-1]
		if !last.End.Equal(parent.End) {
			t.Errorf("the divisions of the %s period do not end with it", parent.Lord)
		}

		for j := range parent.Periods {
			sub := parent.Periods[j]
			if sub.Level != 2 {
				t.Errorf("a division of the %s period is at level %d, want 2", parent.Lord, sub.Level)
			}
			if j > 0 && !sub.Start.Equal(parent.Periods[j-1].End) {
				t.Errorf("division %d of the %s period leaves a gap", j, parent.Lord)
			}
			// Each division takes its own share of a hundred and twenty of
			// the parent.
			want := parent.Years * vimshottariYears[sub.Lord] / VimshottariTotalYears
			if math.Abs(sub.Years-want) > 1e-5 {
				t.Errorf("the %s division of the %s period runs %g years, want %g",
					sub.Lord, parent.Lord, sub.Years, want)
			}
		}
	}

	// The first period keeps only the divisions that had not already passed,
	// and the one birth fell inside starts at birth rather than earlier.
	first := result.Periods[0]
	if len(first.Periods) == 0 {
		t.Fatal("the first period has no divisions at all")
	}
	if first.Periods[0].Start.Before(result.Natal.Moment.UTC) {
		t.Error("a division of the first period begins before the birth")
	}
	if !first.Periods[len(first.Periods)-1].End.Equal(first.End) {
		t.Error("the divisions of the first period do not end with it")
	}
}

// TestDashaYearLengthChangesTheDates checks that the convention is applied and
// is worth reporting: the same birth under two conventions puts the boundaries
// well apart.
func TestDashaYearLengthChangesTheDates(t *testing.T) {
	e := newTestEngine(t)

	julian, err := e.Dashas(context.Background(), dashaRequest(func(r *DashaRequest) {
		r.Depth = 1
		r.YearLength = "julian"
	}))
	if err != nil {
		t.Fatalf("Dashas failed: %v", err)
	}
	savana, err := e.Dashas(context.Background(), dashaRequest(func(r *DashaRequest) {
		r.Depth = 1
		r.YearLength = "savana"
	}))
	if err != nil {
		t.Fatalf("Dashas failed: %v", err)
	}

	if julian.YearLengthDays != 365.25 || savana.YearLengthDays != 360 {
		t.Errorf("the year lengths are %g and %g days, want 365.25 and 360",
			julian.YearLengthDays, savana.YearLengthDays)
	}
	// The same number of years is a shorter stretch of time under the shorter
	// year, so every boundary after the first moves earlier.
	jEnd := julian.Periods[0].End
	sEnd := savana.Periods[0].End
	if !sEnd.Before(jEnd) {
		t.Error("the shorter year did not bring the first boundary forward")
	}
	if gap := jEnd.Sub(sEnd); gap < 24*time.Hour {
		t.Errorf("the two conventions differ by only %v, which is too little to be applied at all", gap)
	}
}

func TestDashaRejectsBadInput(t *testing.T) {
	e := newTestEngine(t)

	cases := []struct {
		name  string
		req   DashaRequest
		field string
	}{
		{
			"nesting deeper than a birth time is known",
			dashaRequest(func(r *DashaRequest) { r.Depth = 6 }),
			"depth",
		},
		{
			"an unknown year convention",
			dashaRequest(func(r *DashaRequest) { r.YearLength = "sidereal" }),
			"year_length",
		},
		{
			"the tropical zodiac, where the mansions do not belong",
			dashaRequest(func(r *DashaRequest) { r.Natal.Settings.Zodiac = "tropical" }),
			"natal.settings.zodiac",
		},
		{
			"a chart without the Moon",
			dashaRequest(func(r *DashaRequest) { r.Natal.Settings.Bodies = []string{"sun"} }),
			"natal.settings.bodies",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.Dashas(context.Background(), tc.req)
			if err == nil {
				t.Fatal("expected an error")
			}
			fe, ok := err.(*tz.FieldError)
			if !ok {
				t.Fatalf("error is %T, want a *tz.FieldError: %v", err, err)
			}
			if fe.Field != tc.field {
				t.Errorf("field = %q, want %q", fe.Field, tc.field)
			}
		})
	}
}

// TestDashaWindowKeepsRealBoundaries checks that narrowing to a span selects
// periods rather than trimming them. A client asking what is running now wants
// the period with its own dates, not one cut off at the edge of the question.
func TestDashaWindowKeepsRealBoundaries(t *testing.T) {
	e := newTestEngine(t)

	from := tz.Input{Date: "2026-01-01", Time: "00:00:00Z"}
	to := tz.Input{Date: "2027-01-01", Time: "00:00:00Z"}

	windowed, err := e.Dashas(context.Background(), dashaRequest(func(r *DashaRequest) {
		r.Depth = 1
		r.From, r.To = &from, &to
	}))
	if err != nil {
		t.Fatalf("Dashas failed: %v", err)
	}
	whole, err := e.Dashas(context.Background(), dashaRequest(func(r *DashaRequest) {
		r.Depth = 1
	}))
	if err != nil {
		t.Fatalf("Dashas failed: %v", err)
	}

	if len(windowed.Periods) == 0 {
		t.Fatal("no period covers 2026")
	}
	if len(windowed.Periods) >= len(whole.Periods) {
		t.Errorf("the window kept %d of %d periods, so it narrowed nothing",
			len(windowed.Periods), len(whole.Periods))
	}

	// Every period kept must match the one in the unwindowed run exactly.
	for _, w := range windowed.Periods {
		var found bool
		for _, p := range whole.Periods {
			if p.Lord == w.Lord && p.Start.Equal(w.Start) && p.End.Equal(w.End) {
				found = true
			}
		}
		if !found {
			t.Errorf("the %s period was trimmed by the window: %v to %v",
				w.Lord, w.Start, w.End)
		}
	}
}
