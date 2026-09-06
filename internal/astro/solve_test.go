package astro

import (
	"context"
	"math"
	"testing"

	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// These techniques all answer "when", so each result can be checked by going
// back to the moment it names and asking whether what was searched for is
// actually there. That is a far stronger test than comparing against a number
// typed into the test.

func TestSolarReturnLandsOnTheNatalDegree(t *testing.T) {
	e := newTestEngine(t)

	result, err := e.Returns(context.Background(), ReturnRequest{
		Natal: referenceRequest,
		Body:  "sun",
		From:  tz.Input{Date: "2024-01-01", Time: "00:00:00Z"},
	})
	if err != nil {
		t.Fatalf("Returns failed: %v", err)
	}
	if len(result.Returns) != 1 {
		t.Fatalf("got %d returns, want 1", len(result.Returns))
	}

	ret := result.Returns[0]
	sun := positionOf(t, ret.Chart, "sun")

	// The whole point of a solar return: at that moment the Sun is back on
	// the degree it held at birth.
	natalSun := positionOf(t, result.Natal, "sun")
	if diff := Separation(sun.Longitude, natalSun.Longitude); diff > 1e-6 {
		t.Errorf("the Sun is %.8f degrees from its natal place at the return", diff)
	}
	if diff := Separation(sun.Longitude, ret.NatalLongitude); diff > 1e-6 {
		t.Errorf("the return is %.8f degrees off the longitude it reports returning to", diff)
	}

	// The Sun comes back to the same degree on the anniversary, give or take
	// a day for the fractional length of the year.
	utc := ret.Chart.Moment.UTC
	if utc.Year() != 2024 {
		t.Errorf("the return is in %d, want the first one after 2024-01-01", utc.Year())
	}
	if utc.Month() != 6 || utc.Day() < 14 || utc.Day() > 16 {
		t.Errorf("the solar return falls on %s, want within a day of 15 June",
			utc.Format("2 January"))
	}
}

func TestLunarReturnsAreOneMonthApart(t *testing.T) {
	e := newTestEngine(t)

	result, err := e.Returns(context.Background(), ReturnRequest{
		Natal: referenceRequest,
		Body:  "moon",
		From:  tz.Input{Date: "2024-01-01", Time: "00:00:00Z"},
		Count: 5,
	})
	if err != nil {
		t.Fatalf("Returns failed: %v", err)
	}
	if len(result.Returns) != 5 {
		t.Fatalf("got %d returns, want 5", len(result.Returns))
	}

	natalMoon := positionOf(t, result.Natal, "moon")
	for i, ret := range result.Returns {
		moon := positionOf(t, ret.Chart, "moon")
		if diff := Separation(moon.Longitude, natalMoon.Longitude); diff > 1e-5 {
			t.Errorf("return %d: the Moon is %.8f degrees from its natal place", i, diff)
		}

		if i > 0 {
			gap := ret.Chart.Moment.JulianDayUT - result.Returns[i-1].Chart.Moment.JulianDayUT
			// The sidereal month is 27.32 days; the gap wanders by a few
			// hours with the Moon's uneven speed.
			if gap < 27.0 || gap > 27.7 {
				t.Errorf("returns %d and %d are %.4f days apart, want about 27.32", i-1, i, gap)
			}
		}
	}
}

func TestReturnsSearchForward(t *testing.T) {
	e := newTestEngine(t)

	result, err := e.Returns(context.Background(), ReturnRequest{
		Natal: referenceRequest,
		Body:  "sun",
		From:  tz.Input{Date: "2024-08-01", Time: "00:00:00Z"},
	})
	if err != nil {
		t.Fatalf("Returns failed: %v", err)
	}

	// The 2024 return is already past on 1 August, so the next one is in
	// 2025. A search that looked backwards would find the wrong year.
	got := result.Returns[0].Chart.Moment.UTC
	if got.Year() != 2025 {
		t.Errorf("the return found is in %d, want 2025", got.Year())
	}
	if got.Before(result.SearchedFrom.UTC) {
		t.Error("a return before the start of the search was returned")
	}
}

func TestReturnsCanUseAnotherLocation(t *testing.T) {
	e := newTestEngine(t)

	london := Location{Latitude: 51.5074, Longitude: -0.1278}
	atHome, err := e.Returns(context.Background(), ReturnRequest{
		Natal: referenceRequest, Body: "sun",
		From: tz.Input{Date: "2024-01-01", Time: "00:00:00Z"},
	})
	if err != nil {
		t.Fatalf("Returns failed: %v", err)
	}
	away, err := e.Returns(context.Background(), ReturnRequest{
		Natal: referenceRequest, Body: "sun", Location: &london,
		From: tz.Input{Date: "2024-01-01", Time: "00:00:00Z"},
	})
	if err != nil {
		t.Fatalf("Returns failed: %v", err)
	}

	// The moment is a fact about the Sun and does not depend on where the
	// chart is cast; the houses very much do.
	if math.Abs(atHome.Returns[0].Chart.Moment.JulianDayUT-
		away.Returns[0].Chart.Moment.JulianDayUT) > 1e-9 {
		t.Error("the moment of the return changed with the location, but it should not")
	}
	homeAsc := atHome.Returns[0].Chart.Houses.Angles[0].Longitude
	awayAsc := away.Returns[0].Chart.Houses.Angles[0].Longitude
	if Separation(homeAsc, awayAsc) < 1 {
		t.Error("the ascendant did not change with the location, but it should have")
	}
}

func TestReturnsRejectBadInput(t *testing.T) {
	e := newTestEngine(t)

	cases := []struct {
		name  string
		req   ReturnRequest
		field string
	}{
		{
			"a body with no return chart",
			ReturnRequest{Natal: referenceRequest, Body: "mars",
				From: tz.Input{Date: "2024-01-01", Time: "00:00Z"}},
			"body",
		},
		{
			"more returns than are sensible",
			ReturnRequest{Natal: referenceRequest, Body: "moon", Count: 500,
				From: tz.Input{Date: "2024-01-01", Time: "00:00Z"}},
			"count",
		},
		{
			"an unreadable start date",
			ReturnRequest{Natal: referenceRequest, Body: "sun",
				From: tz.Input{Date: "2024-01-01", Time: "12:00", Timezone: "Europe/Atlantis"}},
			"from.datetime.timezone",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.Returns(context.Background(), tc.req)
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

// TestSecondaryProgressionCountsADayForAYear checks the rate the technique is
// named for.
func TestSecondaryProgressionCountsADayForAYear(t *testing.T) {
	e := newTestEngine(t)

	result, err := e.Progress(context.Background(), ProgressionRequest{
		Natal:  referenceRequest,
		Target: MomentRequest{DateTime: tz.Input{Date: "2024-06-15", Time: "12:00:00Z"}},
		Method: "secondary",
	})
	if err != nil {
		t.Fatalf("Progress failed: %v", err)
	}

	// From June 1990 to June 2024 is thirty four years.
	if math.Abs(result.ElapsedYears-34) > 0.02 {
		t.Errorf("elapsed years = %.4f, want about 34", result.ElapsedYears)
	}
	if result.ProgressedMoment == nil {
		t.Fatal("a secondary progression is cast for a moment and should report it")
	}

	// Thirty four years of life is thirty four days of ephemeris, so the
	// progressed chart is cast for mid July 1990.
	gap := result.ProgressedMoment.JulianDayUT - result.Natal.Moment.JulianDayUT
	if math.Abs(gap-result.ElapsedYears) > 1e-4 {
		t.Errorf("the progressed moment is %.4f days after the birth, want %.4f",
			gap, result.ElapsedYears)
	}
	if y, m := result.ProgressedMoment.UTC.Year(), result.ProgressedMoment.UTC.Month(); y != 1990 || m != 7 {
		t.Errorf("the progressed chart is cast for %s, want July 1990",
			result.ProgressedMoment.UTC.Format("January 2006"))
	}

	// The progressed Moon covers about a sign a year, so over thirty four
	// years it has gone round more than once.
	natalMoon := positionOf(t, result.Natal, "moon")
	var progressedMoon Position
	for _, p := range result.Positions {
		if p.Body == "moon" {
			progressedMoon = p
		}
	}
	if Separation(natalMoon.Longitude, progressedMoon.Longitude) < 1 {
		t.Error("the progressed Moon has not moved after thirty four years")
	}
}

// TestSolarArcMovesEverythingTogether checks the defining property of the
// technique: one arc for the whole chart.
func TestSolarArcMovesEverythingTogether(t *testing.T) {
	e := newTestEngine(t)

	result, err := e.Progress(context.Background(), ProgressionRequest{
		Natal:  referenceRequest,
		Target: MomentRequest{DateTime: tz.Input{Date: "2024-06-15", Time: "12:00:00Z"}},
		Method: "solar_arc",
	})
	if err != nil {
		t.Fatalf("Progress failed: %v", err)
	}

	// The Sun covers roughly a degree a day, so thirty four years of
	// progression is roughly thirty four degrees.
	if result.Arc < 32 || result.Arc > 36 {
		t.Errorf("arc = %.4f degrees, want about 34", result.Arc)
	}
	if result.ProgressedMoment != nil {
		t.Error("a solar arc is not cast for a moment and should not report one")
	}

	natal := map[string]float64{}
	for _, p := range result.Natal.Positions {
		natal[p.Body] = p.Longitude
	}
	for _, p := range result.Positions {
		want := Normalize(natal[p.Body] + result.Arc)
		if Separation(p.Longitude, want) > 1e-9 {
			t.Errorf("%s moved to %.6f, want %.6f: every point moves by the same arc",
				p.Body, p.Longitude, want)
		}
		// A directed point has no motion of its own.
		if p.Speed != 0 {
			t.Errorf("%s reports a speed of %g, but the whole chart was moved as one",
				p.Body, p.Speed)
		}
	}

	// The angles are directed too, which is the reason the technique is used.
	if result.Houses == nil {
		t.Fatal("the directed angles are missing")
	}
	natalAsc := result.Natal.Houses.Angles[0].Longitude
	directedAsc := result.Houses.Angles[0].Longitude
	if Separation(directedAsc, Normalize(natalAsc+result.Arc)) > 1e-9 {
		t.Error("the ascendant was not directed by the arc")
	}
}

// TestSolarArcPreservesTheChartShape checks a consequence worth stating: since
// everything moves together, the directed chart has exactly the natal aspects.
func TestSolarArcPreservesTheChartShape(t *testing.T) {
	e := newTestEngine(t)

	result, err := e.Progress(context.Background(), ProgressionRequest{
		Natal:  referenceRequest,
		Target: MomentRequest{DateTime: tz.Input{Date: "2024-06-15", Time: "12:00:00Z"}},
		Method: "solar_arc",
	})
	if err != nil {
		t.Fatalf("Progress failed: %v", err)
	}

	directed := FindAspects(result.Positions, result.Settings.AspectTypes, result.Settings.Orbs)
	natalOnly := FindAspects(result.Natal.Positions, result.Settings.AspectTypes, result.Settings.Orbs)

	if len(directed) != len(natalOnly) {
		t.Fatalf("the directed chart has %d aspects and the natal one %d; moving "+
			"everything by the same arc cannot change them", len(directed), len(natalOnly))
	}
	for i := range directed {
		if math.Abs(directed[i].Orb-natalOnly[i].Orb) > 1e-9 {
			t.Errorf("aspect %d has orb %.9f directed and %.9f natally",
				i, directed[i].Orb, natalOnly[i].Orb)
		}
	}
}

func TestProgressionRejectsUnknownMethod(t *testing.T) {
	e := newTestEngine(t)

	_, err := e.Progress(context.Background(), ProgressionRequest{
		Natal:  referenceRequest,
		Target: MomentRequest{DateTime: tz.Input{Date: "2024-06-15", Time: "12:00Z"}},
		Method: "tertiary",
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if fe, ok := err.(*tz.FieldError); !ok || fe.Field != "method" {
		t.Errorf("error = %v, want one naming the method", err)
	}
}

// TestTransitExactTimesAreActuallyExact is the strongest check available for
// the solver: go to the moment it reports and measure the aspect there.
func TestTransitExactTimesAreActuallyExact(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	result, err := e.Transits(ctx, TransitRequest{
		Natal: referenceRequest,
		Transit: MomentRequest{
			DateTime: tz.Input{Date: "2024-03-15", Time: "12:00:00", Timezone: "Europe/Istanbul"},
		},
	})
	if err != nil {
		t.Fatalf("Transits failed: %v", err)
	}

	natal := map[string]float64{}
	for _, p := range natalTargets(result.Natal) {
		natal[p.Body] = p.Longitude
	}

	checked := 0
	for _, a := range result.Aspects {
		if a.ExactAt == nil {
			continue
		}

		// Cast the sky at the moment the solver named.
		at, err := e.Cast(ctx, ChartRequest{
			DateTime: tz.Input{
				Date: a.ExactAt.Format("2006-01-02"),
				Time: a.ExactAt.Format("15:04:05") + "Z",
			},
			Location: referenceRequest.Location,
		})
		if err != nil {
			t.Fatalf("Cast failed: %v", err)
		}

		var moved float64
		var found bool
		for _, p := range at.Positions {
			if p.Body == a.From {
				moved, found = p.Longitude, true
			}
		}
		if !found {
			continue
		}

		// At that moment the two should stand exactly the aspect's angle
		// apart. A second of rounding in the round trip through the calendar
		// lets the fastest bodies drift a little.
		got := Separation(moved, natal[a.To])
		if math.Abs(got-a.Angle) > 0.01 {
			t.Errorf("%s %s %s is exact at %s, but the separation there is %.6f, want %g",
				a.From, a.Type, a.To, a.ExactAt.Format("2006-01-02 15:04:05"), got, a.Angle)
		}
		checked++
	}

	if checked < 5 {
		t.Errorf("only %d aspects carried an exact time, which is too few to have tested anything", checked)
	}
}

// TestExactTimeIsAbsentForStationaryBodies checks that the solver declines
// rather than guessing when a body has no motion to carry it to the aspect.
func TestExactTimeIsAbsentForStationaryBodies(t *testing.T) {
	e := newTestEngine(t)

	result, err := e.Transits(context.Background(), TransitRequest{
		Natal: referenceRequest,
		Transit: MomentRequest{
			DateTime: tz.Input{Date: "2024-03-15", Time: "12:00:00", Timezone: "Europe/Istanbul"},
		},
	})
	if err != nil {
		t.Fatalf("Transits failed: %v", err)
	}

	// Whatever is reported must be a real time; nothing may carry the zero
	// value, which would read as the year one.
	for _, a := range result.Aspects {
		if a.ExactAt != nil && a.ExactAt.Year() < 1800 {
			t.Errorf("%s %s %s reports an exact time of %v",
				a.From, a.Type, a.To, a.ExactAt)
		}
	}
}
