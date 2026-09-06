package astro

import (
	"context"
	"math"
	"testing"

	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

func span(from, to string) RangeRequest {
	return RangeRequest{
		From: tz.Input{Date: from, Time: "00:00:00Z"},
		To:   tz.Input{Date: to, Time: "00:00:00Z"},
	}
}

func TestEphemerisTabulatesPositions(t *testing.T) {
	e := newTestEngine(t)

	result, err := e.Ephemeris(context.Background(), EphemerisRequest{
		RangeRequest: span("2024-01-01", "2024-01-10"),
		Settings:     SettingsInput{Bodies: []string{"sun", "moon"}},
	})
	if err != nil {
		t.Fatalf("Ephemeris failed: %v", err)
	}

	if len(result.Rows) != 10 {
		t.Fatalf("got %d rows, want 10", len(result.Rows))
	}
	for i, row := range result.Rows {
		if len(row.Positions) != 2 {
			t.Errorf("row %d has %d bodies, want 2", i, len(row.Positions))
		}
		if i > 0 {
			gap := row.JulianDayUT - result.Rows[i-1].JulianDayUT
			if math.Abs(gap-1) > 1e-9 {
				t.Errorf("rows %d and %d are %.6f days apart, want 1", i-1, i, gap)
			}
		}
	}

	// The Sun covers about a degree a day, the Moon about thirteen.
	first, last := result.Rows[0], result.Rows[len(result.Rows)-1]
	sunMoved := Normalize(last.Positions[0].Longitude - first.Positions[0].Longitude)
	if sunMoved < 8 || sunMoved > 10 {
		t.Errorf("the Sun moved %.3f degrees in nine days, want about nine", sunMoved)
	}
}

func TestEphemerisRejectsTooManyRows(t *testing.T) {
	e := newTestEngine(t)

	_, err := e.Ephemeris(context.Background(), EphemerisRequest{
		RangeRequest: span("2000-01-01", "2020-01-01"),
		StepDays:     1,
	})
	if err == nil {
		t.Fatal("expected an error; twenty years of daily rows is far past the limit")
	}
	if fe, ok := err.(*tz.FieldError); !ok || fe.Field != "step_days" {
		t.Errorf("error = %v, want one naming step_days", err)
	}
}

func TestRangeIsValidated(t *testing.T) {
	e := newTestEngine(t)

	cases := []struct {
		name  string
		req   RangeRequest
		field string
	}{
		{"backwards", span("2024-06-01", "2024-01-01"), "to"},
		{"empty", span("2024-01-01", "2024-01-01"), "to"},
		{"longer than may be searched", span("1900-01-01", "2000-01-01"), "to"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.MoonPhases(context.Background(), tc.req)
			if err == nil {
				t.Fatal("expected an error")
			}
			if fe, ok := err.(*tz.FieldError); !ok || fe.Field != tc.field {
				t.Errorf("error = %v, want one naming %q", err, tc.field)
			}
		})
	}
}

// TestMoonPhasesAreWhatTheirNamesSay checks each phase against its definition
// rather than against a table of dates: at a new moon the Moon and Sun stand
// together, at a full moon opposite, at the quarters ninety degrees apart.
func TestMoonPhasesAreWhatTheirNamesSay(t *testing.T) {
	e := newTestEngine(t)

	phases, err := e.MoonPhases(context.Background(), span("2024-01-01", "2024-04-01"))
	if err != nil {
		t.Fatalf("MoonPhases failed: %v", err)
	}

	// Three months holds three of each phase, give or take one at the edges.
	if len(phases) < 11 || len(phases) > 13 {
		t.Errorf("got %d phases in three months, want about twelve", len(phases))
	}

	wantElongation := map[MoonPhaseKind]float64{
		PhaseNew: 0, PhaseFirstQuarter: 90, PhaseFull: 180, PhaseLastQuarter: 270,
	}
	for _, p := range phases {
		got := Normalize(p.Moon.Longitude - p.Sun.Longitude)
		want := wantElongation[p.Phase]
		if diff := math.Abs(NormalizeSigned(got - want)); diff > 1e-5 {
			t.Errorf("at the %s of %s the Moon is %.6f degrees from the Sun, want %g",
				p.Phase, p.UTC.Format("2 January"), got, want)
		}
	}

	// They must come out in order and in sequence, each about a week after
	// the last.
	for i := 1; i < len(phases); i++ {
		gap := phases[i].JulianDayUT - phases[i-1].JulianDayUT
		if gap <= 0 {
			t.Fatalf("phase %d is not after phase %d", i, i-1)
		}
		if gap < 5.5 || gap > 9 {
			t.Errorf("phases %d and %d are %.3f days apart, want about seven", i-1, i, gap)
		}
	}
}

// TestKnownNewMoon checks one phase against an independently known date. The
// new moon of 8 April 2024 is the one that darkened North America.
func TestKnownNewMoon(t *testing.T) {
	e := newTestEngine(t)

	phases, err := e.MoonPhases(context.Background(), span("2024-04-05", "2024-04-12"))
	if err != nil {
		t.Fatalf("MoonPhases failed: %v", err)
	}

	var found bool
	for _, p := range phases {
		if p.Phase != PhaseNew {
			continue
		}
		found = true
		if d := p.UTC; d.Month() != 4 || d.Day() != 8 {
			t.Errorf("the new moon is on %s, want 8 April", d.Format("2 January"))
		}
		// It fell at 18:21 UT.
		if h := p.UTC.Hour(); h != 18 {
			t.Errorf("the new moon is at %02d:%02d UT, want about 18:21",
				p.UTC.Hour(), p.UTC.Minute())
		}
	}
	if !found {
		t.Error("no new moon was found in the week around 8 April 2024")
	}
}

// TestMercuryRetrogrades checks the periods against what Mercury actually
// does: three or four times a year, for about three weeks each.
func TestMercuryRetrogrades(t *testing.T) {
	e := newTestEngine(t)

	periods, err := e.Retrogrades(context.Background(), RetrogradeRequest{
		RangeRequest: span("2024-01-01", "2025-01-01"),
		Bodies:       []string{"mercury"},
	})
	if err != nil {
		t.Fatalf("Retrogrades failed: %v", err)
	}

	if len(periods) < 3 || len(periods) > 5 {
		t.Errorf("Mercury retrograded %d times in 2024, want three or four", len(periods))
	}

	complete := 0
	for _, p := range periods {
		if p.Begins == nil || p.Ends == nil {
			continue // runs past the edge of the year
		}
		complete++

		if p.Days < 18 || p.Days > 26 {
			t.Errorf("a retrograde period lasted %.2f days, want about three weeks", p.Days)
		}
		if p.Begins.Kind != StationRetrograde {
			t.Errorf("the period opens with a %s station", p.Begins.Kind)
		}
		if p.Ends.Kind != StationDirect {
			t.Errorf("the period closes with a %s station", p.Ends.Kind)
		}
		if !p.Ends.UTC.After(p.Begins.UTC) {
			t.Error("the period ends before it begins")
		}
		// A station is where the motion stops, so the speed there is nearly
		// nothing.
		if math.Abs(p.Begins.Position.Speed) > 0.01 {
			t.Errorf("the speed at the station is %.6f, want nearly zero",
				p.Begins.Position.Speed)
		}
	}
	if complete < 3 {
		t.Errorf("only %d complete periods were found", complete)
	}
}

// TestRetrogradeAtTheEdgeOfTheSpan checks that a period already under way when
// the span opens is reported with its beginning absent rather than invented.
func TestRetrogradeAtTheEdgeOfTheSpan(t *testing.T) {
	e := newTestEngine(t)

	// Pluto is retrograde for about half of every year, so a span opening in
	// midsummer is certain to start inside one.
	periods, err := e.Retrogrades(context.Background(), RetrogradeRequest{
		RangeRequest: span("2024-07-01", "2024-12-01"),
		Bodies:       []string{"pluto"},
	})
	if err != nil {
		t.Fatalf("Retrogrades failed: %v", err)
	}
	if len(periods) == 0 {
		t.Fatal("Pluto was retrograde through this span and nothing was reported")
	}

	first := periods[0]
	if first.Begins != nil {
		t.Error("the first period claims a beginning, but it started before the span")
	}
	if first.Ends == nil {
		t.Error("the period that was under way has no end, but Pluto turned direct in October")
	}
	if first.Days != 0 {
		t.Errorf("a period with an unknown beginning reports a length of %.2f days", first.Days)
	}
}

func TestRetrogradesRejectBodiesThatNeverTurnBack(t *testing.T) {
	e := newTestEngine(t)

	for _, body := range []string{"sun", "moon"} {
		_, err := e.Retrogrades(context.Background(), RetrogradeRequest{
			RangeRequest: span("2024-01-01", "2024-06-01"),
			Bodies:       []string{body},
		})
		if err == nil {
			t.Errorf("%s never moves backwards and should be refused", body)
			continue
		}
		if fe, ok := err.(*tz.FieldError); !ok || fe.Field != "bodies" {
			t.Errorf("error = %v, want one naming bodies", err)
		}
	}
}

// TestEclipsesOf2024 checks the search against two eclipses everybody knows:
// the total solar eclipse of 8 April 2024 and the annular one of 2 October.
func TestEclipsesOf2024(t *testing.T) {
	e := newTestEngine(t)

	eclipses, err := e.Eclipses(context.Background(), EclipseRequest{
		RangeRequest: span("2024-01-01", "2025-01-01"),
		Kinds:        []string{"solar"},
	})
	if err != nil {
		t.Fatalf("Eclipses failed: %v", err)
	}

	// 2024 had two solar eclipses.
	if len(eclipses) != 2 {
		t.Fatalf("got %d solar eclipses in 2024, want 2", len(eclipses))
	}

	april := eclipses[0]
	if d := april.MaximumUTC; d.Month() != 4 || d.Day() != 8 {
		t.Errorf("the first eclipse is on %s, want 8 April", d.Format("2 January"))
	}
	if april.Type != "total" {
		t.Errorf("the April eclipse is reported as %s, want total", april.Type)
	}
	if !april.Central {
		t.Error("the April eclipse was central and should be reported as such")
	}
	// Greatest eclipse fell over Mexico, so north of the equator and well
	// into the western hemisphere.
	if april.GreatestAt == nil {
		t.Fatal("no place of greatest eclipse was reported")
	}
	if april.GreatestAt.Latitude < 10 || april.GreatestAt.Latitude > 35 {
		t.Errorf("greatest eclipse at latitude %.2f, want the low twenties",
			april.GreatestAt.Latitude)
	}
	if april.GreatestAt.Longitude > -80 || april.GreatestAt.Longitude < -120 {
		t.Errorf("greatest eclipse at longitude %.2f, want about a hundred west",
			april.GreatestAt.Longitude)
	}
	// A total eclipse covers the whole disc.
	if april.Obscuration < 0.99 {
		t.Errorf("obscuration = %.4f for a total eclipse", april.Obscuration)
	}

	october := eclipses[1]
	if d := october.MaximumUTC; d.Month() != 10 || d.Day() != 2 {
		t.Errorf("the second eclipse is on %s, want 2 October", d.Format("2 January"))
	}
	if october.Type != "annular" {
		t.Errorf("the October eclipse is reported as %s, want annular", october.Type)
	}

	// A solar eclipse is a new moon, so the two must be together.
	for _, ec := range eclipses {
		if sep := Separation(ec.Sun.Longitude, ec.Moon.Longitude); sep > 1 {
			t.Errorf("the %s eclipse has the Sun and Moon %.4f degrees apart",
				ec.MaximumUTC.Format("2 January"), sep)
		}
	}
}

// TestLunarEclipsesAreFullMoons checks the other half of the definition.
func TestLunarEclipsesAreFullMoons(t *testing.T) {
	e := newTestEngine(t)

	eclipses, err := e.Eclipses(context.Background(), EclipseRequest{
		RangeRequest: span("2024-01-01", "2025-01-01"),
		Kinds:        []string{"lunar"},
	})
	if err != nil {
		t.Fatalf("Eclipses failed: %v", err)
	}
	if len(eclipses) == 0 {
		t.Fatal("no lunar eclipses were found in 2024")
	}

	for _, ec := range eclipses {
		if ec.Kind != EclipseLunar {
			t.Errorf("a %s eclipse came back from a lunar search", ec.Kind)
		}
		if sep := Separation(ec.Sun.Longitude, ec.Moon.Longitude); math.Abs(sep-180) > 1 {
			t.Errorf("the %s eclipse has the Sun and Moon %.4f degrees apart, want 180",
				ec.MaximumUTC.Format("2 January"), sep)
		}
		// A lunar eclipse looks the same from the whole night side, so it has
		// no place of greatest eclipse.
		if ec.GreatestAt != nil {
			t.Error("a lunar eclipse reported a place of greatest eclipse")
		}
	}
}

func TestEclipsesAreOrderedAndMixed(t *testing.T) {
	e := newTestEngine(t)

	eclipses, err := e.Eclipses(context.Background(), EclipseRequest{
		RangeRequest: span("2024-01-01", "2025-01-01"),
	})
	if err != nil {
		t.Fatalf("Eclipses failed: %v", err)
	}

	kinds := map[EclipseKind]int{}
	for i, ec := range eclipses {
		kinds[ec.Kind]++
		if i > 0 && ec.JulianDayUT < eclipses[i-1].JulianDayUT {
			t.Errorf("eclipse %d comes before eclipse %d", i, i-1)
		}
	}
	if kinds[EclipseSolar] == 0 || kinds[EclipseLunar] == 0 {
		t.Errorf("a search for both kinds returned %v", kinds)
	}
}

// TestRiseSetOrder checks the shape of a day: the Sun rises, climbs to the
// meridian and sets, in that order.
func TestRiseSetOrder(t *testing.T) {
	e := newTestEngine(t)

	result, err := e.RiseSet(context.Background(), RiseSetRequest{
		From:     tz.Input{Date: "2024-06-21", Time: "00:00:00Z"},
		Location: Location{Latitude: 41.0082, Longitude: 28.9784},
		Days:     3,
		Bodies:   []string{"sun"},
	})
	if err != nil {
		t.Fatalf("RiseSet failed: %v", err)
	}
	if len(result.Days) != 3 {
		t.Fatalf("got %d days, want 3", len(result.Days))
	}

	for _, day := range result.Days {
		sun := day.Bodies[0]
		if sun.Rise == nil || sun.Set == nil || sun.Culmination == nil {
			t.Fatalf("%s: the Sun rises and sets in Istanbul in June", day.Date)
		}
		if !sun.Culmination.After(*sun.Rise) {
			t.Errorf("%s: the Sun culminates at %v, before it rises at %v",
				day.Date, sun.Culmination, sun.Rise)
		}
		if !sun.Set.After(*sun.Culmination) {
			t.Errorf("%s: the Sun sets at %v, before it culminates at %v",
				day.Date, sun.Set, sun.Culmination)
		}
		// Around the solstice Istanbul gets a little over fifteen hours of
		// daylight.
		daylight := sun.Set.Sub(*sun.Rise).Hours()
		if daylight < 14.5 || daylight > 15.5 {
			t.Errorf("%s: %.2f hours of daylight, want about fifteen at midsummer",
				day.Date, daylight)
		}
	}
}

// TestMidnightSun checks the polar case: above the Arctic Circle in midsummer
// the Sun never sets, and saying so is more useful than inventing a time.
func TestMidnightSun(t *testing.T) {
	e := newTestEngine(t)

	result, err := e.RiseSet(context.Background(), RiseSetRequest{
		From:     tz.Input{Date: "2024-06-21", Time: "00:00:00Z"},
		Location: Location{Latitude: 78.22, Longitude: 15.63}, // Svalbard
		Bodies:   []string{"sun"},
	})
	if err != nil {
		t.Fatalf("RiseSet failed: %v", err)
	}

	sun := result.Days[0].Bodies[0]
	if sun.Rise != nil || sun.Set != nil {
		t.Errorf("the Sun neither rises nor sets over Svalbard at midsummer, got rise %v set %v",
			sun.Rise, sun.Set)
	}
	// It still crosses the meridian; it simply does so without setting.
	if sun.Culmination == nil {
		t.Error("the Sun still reaches its highest point each day and that should be reported")
	}
	if len(result.Notes) == 0 {
		t.Error("nothing explains why the rise and set are missing")
	}
}

func TestRiseSetRejectsCalculatedPoints(t *testing.T) {
	e := newTestEngine(t)

	_, err := e.RiseSet(context.Background(), RiseSetRequest{
		From:     tz.Input{Date: "2024-06-21", Time: "00:00:00Z"},
		Location: Location{Latitude: 41, Longitude: 29},
		Bodies:   []string{"south_node"},
	})
	if err == nil {
		t.Fatal("a calculated point does not rise or set and should be refused")
	}
	if fe, ok := err.(*tz.FieldError); !ok || fe.Field != "bodies" {
		t.Errorf("error = %v, want one naming bodies", err)
	}
}

// TestObscurationNeverExceedsTheWholeSun guards a quirk of the underlying
// library. For a total or annular eclipse it reports obscuration as the ratio
// of the two discs' areas. In a total eclipse the Moon is the larger, so that
// ratio passes one, which would have the response claim that more than all of
// the Sun was covered.
func TestObscurationNeverExceedsTheWholeSun(t *testing.T) {
	e := newTestEngine(t)

	eclipses, err := e.Eclipses(context.Background(), EclipseRequest{
		RangeRequest: span("2024-01-01", "2028-01-01"),
		Kinds:        []string{"solar"},
	})
	if err != nil {
		t.Fatalf("Eclipses failed: %v", err)
	}

	sawTotal := false
	for _, ec := range eclipses {
		if ec.Obscuration < 0 || ec.Obscuration > 1 {
			t.Errorf("the %s eclipse of %s covers %.4f of the Sun, which is not a fraction of a disc",
				ec.Type, ec.MaximumUTC.Format("2 January 2006"), ec.Obscuration)
		}
		if ec.Type == "total" {
			sawTotal = true
			if ec.Obscuration != 1 {
				t.Errorf("a total eclipse covers %.4f of the Sun, want all of it", ec.Obscuration)
			}
			// The magnitude is the ratio of diameters and does legitimately
			// pass one for a total eclipse; it must not be clamped too.
			if ec.Magnitude <= 1 {
				t.Errorf("a total eclipse has magnitude %.4f, want more than one", ec.Magnitude)
			}
		}
		if ec.Type == "annular" && ec.Obscuration >= 1 {
			t.Errorf("an annular eclipse covers %.4f of the Sun, but a ring is always left",
				ec.Obscuration)
		}
	}
	if !sawTotal {
		t.Error("no total eclipse fell in four years, so the clamp was never exercised")
	}
}
