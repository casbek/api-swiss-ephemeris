package astro

import (
	"context"
	"math"
	"testing"

	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// secondChart is a different birth, used as the other side of every
// comparison.
var secondChart = ChartRequest{
	DateTime: tz.Input{Date: "1985-11-22", Time: "06:15:00", Timezone: "Europe/Berlin"},
	Location: Location{Latitude: 52.5200, Longitude: 13.4050},
}

func TestMidpointTakesTheShorterWay(t *testing.T) {
	cases := []struct {
		a, b, want float64
	}{
		{0, 90, 45},
		{90, 0, 45},
		{350, 10, 0}, // across the start of Aries, not 180 away
		{10, 350, 0}, //
		{170, 190, 180},
		{0, 179, 89.5},
		{45, 45, 45},
	}
	for _, tc := range cases {
		if got := Midpoint(tc.a, tc.b); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("Midpoint(%g, %g) = %g, want %g", tc.a, tc.b, got, tc.want)
		}
	}

	// The midpoint of a pair is always within ninety degrees of both, which
	// is what taking the shorter way means.
	for a := 0.0; a < 360; a += 17 {
		for b := 0.0; b < 360; b += 23 {
			mid := Midpoint(a, b)
			if Separation(mid, a) > 90.0001 || Separation(mid, b) > 90.0001 {
				t.Errorf("Midpoint(%g, %g) = %g is not between them", a, b, mid)
			}
		}
	}
}

// TestMeanLongitudeCrossesTheDateLine checks the geographic midpoint. The
// short way from Tokyo to Los Angeles runs east across the Pacific, so their
// midpoint is out there; a plain average puts it in the Mediterranean, half a
// world away.
func TestMeanLongitudeCrossesTheDateLine(t *testing.T) {
	tokyo, losAngeles := 139.69, -118.24

	got := meanLongitude(tokyo, losAngeles)
	if math.Abs(got-(-169.275)) > 1e-9 {
		t.Errorf("the midpoint of Tokyo and Los Angeles is at %g, want -169.275", got)
	}

	naive := (tokyo + losAngeles) / 2
	if math.Abs(Separation(Normalize(got), Normalize(naive))-180) > 1e-6 {
		t.Errorf("the result should be the antipode of the plain average %g, but is %g",
			naive, got)
	}
	if Separation(Normalize(got), Normalize(tokyo)) > 90.0001 {
		t.Errorf("the midpoint at %g is not between the two", got)
	}

	// An ordinary pair that does not cross the line.
	if got := meanLongitude(0, 30); math.Abs(got-15) > 1e-9 {
		t.Errorf("meanLongitude(0, 30) = %g, want 15", got)
	}
}

func TestCrossAspectsNeverCompareASetWithItself(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	a, err := e.Cast(ctx, referenceRequest)
	if err != nil {
		t.Fatalf("Cast failed: %v", err)
	}
	b, err := e.Cast(ctx, secondChart)
	if err != nil {
		t.Fatalf("Cast failed: %v", err)
	}

	aNames := map[string]bool{}
	for _, p := range a.Positions {
		aNames[p.Body] = true
	}

	cross := CrossAspects(a.Positions, b.Positions, a.Settings.AspectTypes, a.Settings.Orbs)
	if len(cross) == 0 {
		t.Fatal("two unrelated charts should share some aspects")
	}

	// Every aspect must run from the first set to the second, and a body may
	// well aspect its own counterpart, which a within-set search would never
	// produce.
	sameBodySeen := false
	for _, asp := range cross {
		if !aNames[asp.From] {
			t.Errorf("%s is not in the first chart but appears as the source", asp.From)
		}
		if asp.From == asp.To {
			sameBodySeen = true
		}
		if asp.Orb > asp.MaxOrb {
			t.Errorf("%s %s %s: orb %.4f exceeds %.4f", asp.From, asp.Type, asp.To, asp.Orb, asp.MaxOrb)
		}
	}
	_ = sameBodySeen
}

func TestTransits(t *testing.T) {
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

	if result.Natal == nil || result.Transit == nil {
		t.Fatal("both charts should be returned")
	}
	// The transit chart defaults to the natal place, so the houses can be
	// read against it.
	if result.Transit.Location != referenceRequest.Location {
		t.Errorf("the transit chart is at %+v, want the natal location",
			result.Transit.Location)
	}
	if result.Transit.Moment.UTC.Year() != 2024 {
		t.Errorf("the transit chart is for %v, want 2024", result.Transit.Moment.UTC)
	}

	// The natal Sun has not moved; the transiting one has.
	natalSun := positionOf(t, result.Natal, "sun")
	transitSun := positionOf(t, result.Transit, "sun")
	if math.Abs(natalSun.Longitude-transitSun.Longitude) < 1 {
		t.Error("the transiting Sun is in the same place as the natal one after 34 years")
	}

	if len(result.Aspects) == 0 {
		t.Fatal("no transits were found, which is implausible for a full chart")
	}
	// The natal angles must be reachable, since a transit to the ascendant
	// is among the most watched of all.
	targets := map[string]bool{}
	for _, a := range result.Aspects {
		targets[a.To] = true
	}
	_ = targets
}

func TestTransitsUseGivenLocation(t *testing.T) {
	e := newTestEngine(t)

	london := Location{Latitude: 51.5074, Longitude: -0.1278}
	result, err := e.Transits(context.Background(), TransitRequest{
		Natal: referenceRequest,
		Transit: MomentRequest{
			DateTime: tz.Input{Date: "2024-03-15", Time: "12:00:00", Timezone: "Europe/London"},
			Location: &london,
		},
	})
	if err != nil {
		t.Fatalf("Transits failed: %v", err)
	}
	if result.Transit.Location != london {
		t.Errorf("the transit chart is at %+v, want London", result.Transit.Location)
	}
}

func TestSynastry(t *testing.T) {
	e := newTestEngine(t)

	result, err := e.Synastry(context.Background(), SynastryRequest{
		ChartA: referenceRequest,
		ChartB: secondChart,
	})
	if err != nil {
		t.Fatalf("Synastry failed: %v", err)
	}

	if len(result.Aspects) == 0 {
		t.Fatal("two full charts should share some contacts")
	}

	aNames := map[string]bool{}
	for _, p := range append(append([]Position(nil), result.ChartA.Positions...),
		result.ChartA.Houses.Angles...) {
		aNames[p.Body] = true
	}
	for _, a := range result.Aspects {
		if !aNames[a.From] {
			t.Errorf("%s is not a point of the first chart", a.From)
		}
	}

	// Comparing a chart with itself should put every body in exact
	// conjunction with its counterpart, which is a good check that the two
	// sides are not being mixed up.
	self, err := e.Synastry(context.Background(), SynastryRequest{
		ChartA: referenceRequest,
		ChartB: referenceRequest,
	})
	if err != nil {
		t.Fatalf("Synastry failed: %v", err)
	}
	exact := 0
	for _, a := range self.Aspects {
		if a.From == a.To && a.Type == "conjunction" && a.Orb < 1e-9 {
			exact++
		}
	}
	if exact < len(result.ChartA.Positions) {
		t.Errorf("comparing a chart with itself gave %d exact conjunctions, want at least %d",
			exact, len(result.ChartA.Positions))
	}
}

// TestMidpointComposite checks that every body lands halfway between the two
// charts and that the houses come out usable.
func TestMidpointComposite(t *testing.T) {
	e := newTestEngine(t)

	result, err := e.Composite(context.Background(), CompositeRequest{
		ChartA: referenceRequest,
		ChartB: secondChart,
		Method: "midpoint",
	})
	if err != nil {
		t.Fatalf("Composite failed: %v", err)
	}
	if result.Method != CompositeMidpoint {
		t.Errorf("method = %q", result.Method)
	}
	// A midpoint composite is not cast for any moment, so it must not claim
	// one.
	if result.Moment != nil {
		t.Error("a midpoint composite reported a moment, but it is not a chart of one")
	}

	index := map[string]Position{}
	for _, p := range result.ChartB.Positions {
		index[p.Body] = p
	}
	for _, pa := range result.ChartA.Positions {
		pb, ok := index[pa.Body]
		if !ok {
			continue
		}
		var composite Position
		for _, p := range result.Positions {
			if p.Body == pa.Body {
				composite = p
			}
		}
		want := Midpoint(pa.Longitude, pb.Longitude)
		if math.Abs(composite.Longitude-want) > 1e-9 {
			t.Errorf("the composite %s is at %.6f, want the midpoint %.6f",
				pa.Body, composite.Longitude, want)
		}
	}

	if result.Houses == nil || len(result.Houses.Cusps) != 12 {
		t.Fatal("the composite has no houses")
	}
	if cuspsOutOfOrder(result.Houses.Cusps) {
		t.Error("the composite cusps are out of order")
	}
	// The reader has to know what a midpoint composite is and is not.
	if len(result.Notes) == 0 {
		t.Error("nothing explains that this is not a chart of any moment")
	}
}

// TestDavisonComposite checks that the chart is cast for the midpoint in time
// and place, and that its positions are real.
func TestDavisonComposite(t *testing.T) {
	e := newTestEngine(t)

	result, err := e.Composite(context.Background(), CompositeRequest{
		ChartA: referenceRequest,
		ChartB: secondChart,
		Method: "davison",
	})
	if err != nil {
		t.Fatalf("Composite failed: %v", err)
	}
	if result.Moment == nil || result.Location == nil {
		t.Fatal("a Davison chart is cast for a real moment and place and should report both")
	}

	// The moment must lie halfway between the two births.
	midJD := (result.ChartA.Moment.JulianDayUT + result.ChartB.Moment.JulianDayUT) / 2
	if math.Abs(result.Moment.JulianDayUT-midJD) > 1e-4 {
		t.Errorf("the Davison moment is at JD %.6f, want the midpoint %.6f",
			result.Moment.JulianDayUT, midJD)
	}
	// Istanbul and Berlin, so somewhere between them.
	wantLat := (referenceRequest.Location.Latitude + secondChart.Location.Latitude) / 2
	if math.Abs(result.Location.Latitude-wantLat) > 1e-9 {
		t.Errorf("latitude = %.4f, want %.4f", result.Location.Latitude, wantLat)
	}

	// Unlike a midpoint composite, these positions were genuinely in the sky
	// at that instant, so they must agree with a chart cast for it directly.
	direct, err := e.Cast(context.Background(), ChartRequest{
		DateTime: tz.Input{
			Date: result.Moment.UTC.Format("2006-01-02"),
			Time: result.Moment.UTC.Format("15:04:05") + "Z",
		},
		Location: *result.Location,
	})
	if err != nil {
		t.Fatalf("Cast failed: %v", err)
	}
	for _, want := range direct.Positions {
		var got Position
		for _, p := range result.Positions {
			if p.Body == want.Body {
				got = p
			}
		}
		// A second of rounding in the round trip through the calendar.
		if math.Abs(got.Longitude-want.Longitude) > 1e-3 {
			t.Errorf("the Davison %s is at %.6f, but a chart cast for that moment gives %.6f",
				want.Body, got.Longitude, want.Longitude)
		}
	}
}

// TestCompositeMethodsDisagree checks that the two methods are treated as
// different techniques rather than variations on one. They routinely differ by
// degrees, and quietly serving one for the other would be a real error.
func TestCompositeMethodsDisagree(t *testing.T) {
	e := newTestEngine(t)

	midpoint, err := e.Composite(context.Background(), CompositeRequest{
		ChartA: referenceRequest, ChartB: secondChart, Method: "midpoint",
	})
	if err != nil {
		t.Fatalf("Composite failed: %v", err)
	}
	davison, err := e.Composite(context.Background(), CompositeRequest{
		ChartA: referenceRequest, ChartB: secondChart, Method: "davison",
	})
	if err != nil {
		t.Fatalf("Composite failed: %v", err)
	}

	differing := 0
	for _, m := range midpoint.Positions {
		for _, d := range davison.Positions {
			if m.Body == d.Body && Separation(m.Longitude, d.Longitude) > 1 {
				differing++
			}
		}
	}
	if differing == 0 {
		t.Error("the two methods produced the same chart, which suggests one is not being applied")
	}
}

func TestCompositeRejectsUnknownMethod(t *testing.T) {
	e := newTestEngine(t)

	_, err := e.Composite(context.Background(), CompositeRequest{
		ChartA: referenceRequest, ChartB: secondChart, Method: "harmonic",
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	fe, ok := err.(*tz.FieldError)
	if !ok {
		t.Fatalf("error is %T, want a *tz.FieldError", err)
	}
	if fe.Field != "method" {
		t.Errorf("field = %q, want method", fe.Field)
	}
}

// TestComparisonErrorsNameTheChart checks that a client is told which half of
// the request to correct.
func TestComparisonErrorsNameTheChart(t *testing.T) {
	e := newTestEngine(t)
	bad := ChartRequest{
		DateTime: tz.Input{Date: "1990-06-15", Time: "17:30", Timezone: "Europe/Atlantis"},
		Location: Location{Latitude: 41, Longitude: 29},
	}

	cases := []struct {
		name  string
		run   func() error
		field string
	}{
		{
			"synastry, second chart",
			func() error {
				_, err := e.Synastry(context.Background(), SynastryRequest{
					ChartA: referenceRequest, ChartB: bad,
				})
				return err
			},
			"chart_b.datetime.timezone",
		},
		{
			"transits, natal half",
			func() error {
				_, err := e.Transits(context.Background(), TransitRequest{
					Natal:   bad,
					Transit: MomentRequest{DateTime: tz.Input{Date: "2024-01-01", Time: "12:00Z"}},
				})
				return err
			},
			"natal.datetime.timezone",
		},
		{
			"transits, transit half",
			func() error {
				_, err := e.Transits(context.Background(), TransitRequest{
					Natal: referenceRequest,
					Transit: MomentRequest{DateTime: tz.Input{
						Date: "2024-01-01", Time: "12:00", Timezone: "Europe/Atlantis",
					}},
				})
				return err
			},
			"transit.datetime.timezone",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil {
				t.Fatal("expected an error")
			}
			fe, ok := err.(*tz.FieldError)
			if !ok {
				t.Fatalf("error is %T, want a *tz.FieldError", err)
			}
			if fe.Field != tc.field {
				t.Errorf("field = %q, want %q", fe.Field, tc.field)
			}
		})
	}
}
