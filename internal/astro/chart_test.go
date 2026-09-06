package astro

import (
	"context"
	"math"
	"testing"

	"github.com/casbek/api-swiss-ephemeris/internal/swe"
	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// The reference chart used throughout: 15 June 1990, 17:30 local in Istanbul,
// which is 14:30 UT. Its planetary longitudes and Placidus cusps were produced
// with swetest and are asserted in internal/swe.
var referenceRequest = ChartRequest{
	DateTime: tz.Input{Date: "1990-06-15", Time: "17:30:00", Timezone: "Europe/Istanbul"},
	Location: Location{Latitude: 41.0082, Longitude: 28.9784},
}

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	calc, err := swe.New(swe.Config{EphePath: "../../ephe"})
	if err != nil {
		t.Fatalf("could not start calculator: %v", err)
	}
	t.Cleanup(calc.Close)
	return NewEngine(calc)
}

func castReference(t *testing.T, adjust func(*ChartRequest)) *Chart {
	t.Helper()
	req := referenceRequest
	if adjust != nil {
		adjust(&req)
	}
	chart, err := (newTestEngine(t)).Cast(context.Background(), req)
	if err != nil {
		t.Fatalf("Cast failed: %v", err)
	}
	return chart
}

func positionOf(t *testing.T, c *Chart, body string) Position {
	t.Helper()
	for _, p := range c.Positions {
		if p.Body == body {
			return p
		}
	}
	t.Fatalf("%s is not in the chart", body)
	return Position{}
}

func TestChartPositionsMatchReference(t *testing.T) {
	c := castReference(t, nil)

	want := map[string]struct {
		longitude float64
		sign      string
		retro     bool
	}{
		"sun":     {84.2290447, "gemini", false},
		"moon":    {346.7518340, "pisces", false},
		"mercury": {65.8696941, "gemini", false},
		"venus":   {48.9001012, "taurus", false},
		"mars":    {11.1161937, "aries", false},
		"jupiter": {105.9120560, "cancer", false},
		"saturn":  {294.0258212, "capricorn", true},
		"uranus":  {278.1611546, "capricorn", true},
		"neptune": {283.7144691, "capricorn", true},
		"pluto":   {225.3994063, "scorpio", true},
	}

	// The reference values were produced by swetest, which takes its -ut
	// argument as Universal Time directly. This path instead converts UTC
	// through swe_utc_to_jd, which accounts for leap seconds and yields UT1,
	// the scale swe_calc_ut actually expects. The two differ by about 18
	// milliseconds in 1990, which moves the Moon by three millionths of a
	// degree and everything slower by less. The tolerance allows for that.
	const bodyTolerance = 1e-5

	for body, w := range want {
		p := positionOf(t, c, body)
		if diff := math.Abs(p.Longitude - w.longitude); diff > bodyTolerance {
			t.Errorf("%s longitude = %.7f, want %.7f", body, p.Longitude, w.longitude)
		}
		if p.Sign != w.sign {
			t.Errorf("%s sign = %q, want %q", body, p.Sign, w.sign)
		}
		if p.IsRetrograde != w.retro {
			t.Errorf("%s retrograde = %v, want %v", body, p.IsRetrograde, w.retro)
		}
		// The degree within the sign must agree with the longitude.
		if diff := math.Abs(p.DegreeInSign - math.Mod(p.Longitude, 30)); diff > 1e-9 {
			t.Errorf("%s degree_in_sign = %g does not follow from longitude %g",
				body, p.DegreeInSign, p.Longitude)
		}
	}
}

// TestChartHouseAssignment checks bodies against the Placidus cusps, which
// were computed by hand from the reference cusps.
func TestChartHouseAssignment(t *testing.T) {
	c := castReference(t, nil)

	if c.Houses == nil {
		t.Fatal("no houses were calculated")
	}
	if len(c.Houses.Cusps) != 12 {
		t.Fatalf("got %d cusps, want 12", len(c.Houses.Cusps))
	}

	want := map[string]int{
		"sun": 8, "moon": 4, "mercury": 7, "venus": 7, "mars": 5,
		"jupiter": 8, "saturn": 3, "uranus": 2, "neptune": 2, "pluto": 12,
	}
	for body, wantHouse := range want {
		p := positionOf(t, c, body)
		if p.House != wantHouse {
			t.Errorf("%s is in house %d, want %d (longitude %.4f)",
				body, p.House, wantHouse, p.Longitude)
		}
		// The fractional position must sit inside its own house.
		if p.HousePosition < float64(p.House) || p.HousePosition >= float64(p.House)+1 {
			t.Errorf("%s house_position = %g, outside house %d",
				body, p.HousePosition, p.House)
		}
	}
}

func TestChartAngles(t *testing.T) {
	c := castReference(t, nil)

	angles := map[string]float64{}
	for _, a := range c.Houses.Angles {
		angles[a.Body] = a.Longitude
	}

	// The angles turn a full circle a day, so the 18 millisecond difference
	// between the reference time scale and this one moves them by about
	// eight hundredths of a millidegree. See TestChartPositionsMatchReference.
	const angleTolerance = 2e-4

	if diff := math.Abs(angles["ascendant"] - 227.1761062); diff > angleTolerance {
		t.Errorf("ascendant = %.7f, want 227.1761062", angles["ascendant"])
	}
	if diff := math.Abs(angles["midheaven"] - 147.9146197); diff > angleTolerance {
		t.Errorf("midheaven = %.7f, want 147.9146197", angles["midheaven"])
	}
	// The descendant and imum coeli are opposite the other two by definition.
	if diff := math.Abs(Normalize(angles["descendant"]-angles["ascendant"]) - 180); diff > 1e-9 {
		t.Error("the descendant is not opposite the ascendant")
	}
	if diff := math.Abs(Normalize(angles["imum_coeli"]-angles["midheaven"]) - 180); diff > 1e-9 {
		t.Error("the imum coeli is not opposite the midheaven")
	}
}

// TestChartSect checks the day or night reading. The chart is for half past
// five on a June afternoon in Istanbul, so the Sun is well above the horizon
// and it is unambiguously a day chart.
func TestChartSect(t *testing.T) {
	c := castReference(t, nil)
	if c.Sect != SectDay {
		t.Errorf("sect = %q, want day; the Sun is in house %d",
			c.Sect, positionOf(t, c, "sun").House)
	}
}

// TestChartDignities checks placements worked out by hand from the tables.
func TestChartDignities(t *testing.T) {
	c := castReference(t, nil)

	// Mars is in Aries, the sign it rules.
	mars := positionOf(t, c, "mars")
	if mars.Dignity == nil {
		t.Fatal("Mars has no dignity")
	}
	if !mars.Dignity.Domicile {
		t.Error("Mars in Aries should be in domicile")
	}
	if mars.Dignity.Score != ScoreDomicile {
		t.Errorf("Mars score = %d, want %d", mars.Dignity.Score, ScoreDomicile)
	}

	// Saturn is at 24 Capricorn: its own sign, and its own Egyptian bound,
	// which runs from 22 to 26 degrees.
	saturn := positionOf(t, c, "saturn")
	if !saturn.Dignity.Domicile {
		t.Error("Saturn in Capricorn should be in domicile")
	}
	if !saturn.Dignity.InOwnTerm || saturn.Dignity.TermRuler != "saturn" {
		t.Errorf("Saturn at %.2f Capricorn should be in its own bound, got %q",
			saturn.DegreeInSign, saturn.Dignity.TermRuler)
	}
	if want := ScoreDomicile + ScoreTerm; saturn.Dignity.Score != want {
		t.Errorf("Saturn score = %d, want %d", saturn.Dignity.Score, want)
	}

	// The Sun is at 24 Gemini, the third decan, which the Chaldean sequence
	// gives to the Sun itself.
	sun := positionOf(t, c, "sun")
	if !sun.Dignity.InOwnFace || sun.Dignity.FaceRuler != "sun" {
		t.Errorf("the Sun in the third decan of Gemini should rule its own face, got %q",
			sun.Dignity.FaceRuler)
	}
	if sun.Dignity.Score != ScoreFace {
		t.Errorf("Sun score = %d, want %d", sun.Dignity.Score, ScoreFace)
	}

	// The Moon is in Pisces, a water sign, and participates in the water
	// triplicity in both sects.
	moon := positionOf(t, c, "moon")
	if !moon.Dignity.Triplicity {
		t.Error("the Moon in Pisces should have the water triplicity")
	}
	if moon.Dignity.Score != ScoreTriplicity {
		t.Errorf("Moon score = %d, want %d", moon.Dignity.Score, ScoreTriplicity)
	}

	// Dignity is a scheme for the seven classical planets only.
	for _, body := range []string{"uranus", "neptune", "pluto", "chiron", "true_node", "lilith"} {
		if p := positionOf(t, c, body); p.Dignity != nil {
			t.Errorf("%s should have no essential dignity", body)
		}
	}
}

// TestChartAspects checks a specific aspect worked out from the reference
// longitudes, and the invariants that must hold for all of them.
func TestChartAspects(t *testing.T) {
	c := castReference(t, nil)

	var sunMoon *Aspect
	for i := range c.Aspects {
		a := c.Aspects[i]
		if (a.From == "sun" && a.To == "moon") || (a.From == "moon" && a.To == "sun") {
			sunMoon = &c.Aspects[i]
		}
	}
	if sunMoon == nil {
		t.Fatal("the Sun and Moon are 97.48 degrees apart and should form a square")
	}
	if sunMoon.Type != "square" {
		t.Errorf("Sun to Moon is a %s, want a square", sunMoon.Type)
	}
	if diff := math.Abs(sunMoon.Orb - 7.4772); diff > 1e-3 {
		t.Errorf("orb = %.4f, want 7.4772", sunMoon.Orb)
	}
	// The Moon moves at thirteen degrees a day against the Sun's one, so it
	// is closing on the square.
	if !sunMoon.IsApplying {
		t.Error("the Moon is closing on the square and the aspect should be applying")
	}

	for _, a := range c.Aspects {
		if a.Orb > a.MaxOrb {
			t.Errorf("%s %s %s: orb %.4f exceeds the allowed %.4f",
				a.From, a.Type, a.To, a.Orb, a.MaxOrb)
		}
		if a.From == a.To {
			t.Errorf("%s aspects itself", a.From)
		}
		if diff := math.Abs(math.Abs(a.Separation-a.Angle) - a.Orb); diff > 1e-9 {
			t.Errorf("%s %s %s: orb %.6f does not follow from separation %.6f and angle %g",
				a.From, a.Type, a.To, a.Orb, a.Separation, a.Angle)
		}
	}

	// Tightest first.
	for i := 1; i < len(c.Aspects); i++ {
		if c.Aspects[i].Orb < c.Aspects[i-1].Orb {
			t.Errorf("aspects are not ordered by orb at index %d", i)
		}
	}
}

// TestChartOnlyOneAspectPerPair checks that widening the orbs does not produce
// a pair reported as making two different aspects at once.
func TestChartOnlyOneAspectPerPair(t *testing.T) {
	c := castReference(t, func(r *ChartRequest) {
		r.Settings.Aspects = &AspectInput{
			Types: []string{"conjunction", "opposition", "trine", "square",
				"sextile", "quincunx", "semisextile", "semisquare", "sesquiquadrate"},
			Orbs: map[string]float64{"sun": 25, "moon": 25},
		}
	})

	seen := map[string]string{}
	for _, a := range c.Aspects {
		key := a.From + "|" + a.To
		if prev, dup := seen[key]; dup {
			t.Errorf("%s and %s are reported as both %s and %s", a.From, a.To, prev, a.Type)
		}
		seen[key] = a.Type
	}
}

func TestChartDeclination(t *testing.T) {
	c := castReference(t, nil)

	// Mid June is close to the solstice, so the Sun is near its greatest
	// northern declination, which is the obliquity itself.
	sun := positionOf(t, c, "sun")
	if sun.Declination < 23.0 || sun.Declination > 23.45 {
		t.Errorf("the Sun's declination is %.4f, want roughly 23.3 in mid June", sun.Declination)
	}
	if c.Obliquity < 23.4 || c.Obliquity > 23.5 {
		t.Errorf("obliquity = %.4f, want roughly 23.44", c.Obliquity)
	}
	// No body can be further from the equator than the obliquity plus its
	// own ecliptic latitude.
	for _, p := range c.Positions {
		if math.Abs(p.Declination) > c.Obliquity+math.Abs(p.Latitude)+1e-6 {
			t.Errorf("%s declination %.4f is impossible for latitude %.4f",
				p.Body, p.Declination, p.Latitude)
		}
	}
}

func TestChartDistribution(t *testing.T) {
	c := castReference(t, nil)

	total := 0
	for _, n := range c.Distribution.Elements {
		total += n
	}
	if total != len(c.Positions) {
		t.Errorf("the elements account for %d bodies, but the chart has %d", total, len(c.Positions))
	}

	total = 0
	for _, n := range c.Distribution.Modalities {
		total += n
	}
	if total != len(c.Positions) {
		t.Errorf("the modalities account for %d bodies, but the chart has %d", total, len(c.Positions))
	}
}

// TestChartWithUnknownTimeOmitsHouses checks that nothing depending on the
// time of day is invented when the time was not given.
func TestChartWithUnknownTimeOmitsHouses(t *testing.T) {
	c := castReference(t, func(r *ChartRequest) { r.DateTime.Time = "" })

	if c.Houses != nil {
		t.Error("houses were calculated for a chart with an unknown birth time")
	}
	if c.Sect != "" {
		t.Errorf("sect = %q, but it cannot be known without the time", c.Sect)
	}
	for _, p := range c.Positions {
		if p.House != 0 {
			t.Errorf("%s was assigned house %d without a known birth time", p.Body, p.House)
		}
	}
	// Positions themselves are still meaningful, bar the fast moving Moon.
	if len(c.Positions) == 0 {
		t.Error("no positions were calculated")
	}
}

// TestSiderealChart checks that the sidereal zodiac shifts every position by
// the ayanamsha and nothing else.
func TestSiderealChart(t *testing.T) {
	tropical := castReference(t, nil)
	sidereal := castReference(t, func(r *ChartRequest) {
		r.Settings.Zodiac = "sidereal"
		r.Settings.Ayanamsha = "lahiri"
	})

	if sidereal.Ayanamsha < 23 || sidereal.Ayanamsha > 24.5 {
		t.Errorf("ayanamsha = %.4f, want roughly 23.7 for 1990", sidereal.Ayanamsha)
	}

	for _, tp := range tropical.Positions {
		var sp Position
		for _, p := range sidereal.Positions {
			if p.Body == tp.Body {
				sp = p
			}
		}
		shift := Normalize(tp.Longitude - sp.Longitude)
		if diff := math.Abs(shift - sidereal.Ayanamsha); diff > 1e-6 {
			t.Errorf("%s moved by %.6f, want the ayanamsha %.6f",
				tp.Body, shift, sidereal.Ayanamsha)
		}
	}

	// Subtracting the ayanamsha moves the Sun from 24 Gemini back to the
	// very start of Gemini, half a degree from the Taurus boundary. A chart
	// this close to a cusp is exactly where the two zodiacs are most often
	// confused for one another.
	sun := positionOf(t, sidereal, "sun")
	if sun.Sign != "gemini" {
		t.Errorf("the sidereal Sun is in %s, want gemini", sun.Sign)
	}
	if sun.DegreeInSign > 1 {
		t.Errorf("the sidereal Sun is at %.4f into Gemini, want under a degree",
			sun.DegreeInSign)
	}
}

func TestChartRejectsBadSettings(t *testing.T) {
	cases := []struct {
		name  string
		req   ChartRequest
		field string
	}{
		{
			name: "unknown body",
			req: ChartRequest{
				DateTime: referenceRequest.DateTime, Location: referenceRequest.Location,
				Settings: SettingsInput{Bodies: []string{"sun", "nibiru"}},
			},
			field: "settings.bodies",
		},
		{
			name: "an angle asked for as a body",
			req: ChartRequest{
				DateTime: referenceRequest.DateTime, Location: referenceRequest.Location,
				Settings: SettingsInput{Bodies: []string{"ascendant"}},
			},
			field: "settings.bodies",
		},
		{
			name: "unknown house system",
			req: ChartRequest{
				DateTime: referenceRequest.DateTime, Location: referenceRequest.Location,
				Settings: SettingsInput{HouseSystem: "ptolemaic"},
			},
			field: "settings.house_system",
		},
		{
			name: "unknown zodiac",
			req: ChartRequest{
				DateTime: referenceRequest.DateTime, Location: referenceRequest.Location,
				Settings: SettingsInput{Zodiac: "draconic"},
			},
			field: "settings.zodiac",
		},
		{
			name: "orb beyond what means anything",
			req: ChartRequest{
				DateTime: referenceRequest.DateTime, Location: referenceRequest.Location,
				Settings: SettingsInput{Aspects: &AspectInput{Orbs: map[string]float64{"sun": 90}}},
			},
			field: "settings.aspects.orbs",
		},
		{
			name: "impossible latitude",
			req: ChartRequest{
				DateTime: referenceRequest.DateTime,
				Location: Location{Latitude: 100, Longitude: 28},
			},
			field: "location.latitude",
		},
		{
			// Placidus is undefined inside the Arctic Circle, where the
			// ecliptic can fail to cross the horizon as it requires.
			name: "Placidus above the Arctic Circle",
			req: ChartRequest{
				DateTime: referenceRequest.DateTime,
				Location: Location{Latitude: 78.2, Longitude: 15.6},
			},
			field: "settings.house_system",
		},
	}

	e := newTestEngine(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.Cast(context.Background(), tc.req)
			if err == nil {
				t.Fatal("expected an error")
			}
			fe, ok := err.(*tz.FieldError)
			if !ok {
				t.Fatalf("error is %T, want a *tz.FieldError: %v", err, err)
			}
			if fe.Field != tc.field {
				t.Errorf("field = %q, want %q (%v)", fe.Field, tc.field, err)
			}
		})
	}
}

// TestWholeSignWorksAtHighLatitude checks that a chart inside the Arctic
// Circle is refused only for the systems that cannot handle it.
func TestWholeSignWorksAtHighLatitude(t *testing.T) {
	c := castReference(t, func(r *ChartRequest) {
		r.Location = Location{Latitude: 78.2, Longitude: 15.6}
		r.Settings.HouseSystem = "whole_sign"
	})

	if c.Houses == nil || len(c.Houses.Cusps) != 12 {
		t.Fatal("whole sign houses should be available at any latitude")
	}
	// Whole sign cusps always fall on the first degree of a sign.
	for _, cusp := range c.Houses.Cusps {
		if math.Abs(cusp.DegreeInSign) > 1e-9 {
			t.Errorf("house %d starts at %.6f into %s, but whole sign cusps start a sign",
				cusp.House, cusp.DegreeInSign, cusp.Sign)
		}
	}
}

func TestSouthNodeIsOppositeNorth(t *testing.T) {
	c := castReference(t, func(r *ChartRequest) {
		r.Settings.Bodies = []string{"true_node", "south_node"}
	})

	north := positionOf(t, c, "true_node")
	south := positionOf(t, c, "south_node")

	if diff := math.Abs(Normalize(south.Longitude-north.Longitude) - 180); diff > 1e-9 {
		t.Errorf("the south node is %.6f from the north node, want 180",
			Normalize(south.Longitude-north.Longitude))
	}
	if south.Speed != north.Speed {
		t.Error("the nodes share their motion and should report the same speed")
	}
}

// TestNoTautologousAspects checks that points which are opposite by
// definition are not reported as aspecting one another. The descendant is the
// degree opposite the ascendant, so an exact opposition between them holds in
// every chart and would head the list of every response while saying nothing.
func TestNoTautologousAspects(t *testing.T) {
	c := castReference(t, func(r *ChartRequest) {
		r.Settings.Bodies = []string{"sun", "moon", "true_node", "south_node"}
	})

	forbidden := [][2]string{
		{"ascendant", "descendant"},
		{"midheaven", "imum_coeli"},
		{"true_node", "south_node"},
	}
	for _, a := range c.Aspects {
		for _, pair := range forbidden {
			if (a.From == pair[0] && a.To == pair[1]) || (a.From == pair[1] && a.To == pair[0]) {
				t.Errorf("%s and %s are opposite by definition but were reported as a %s",
					a.From, a.To, a.Type)
			}
		}
	}

	// Aspects between the angles that are not definitional still belong. The
	// angle between the ascendant and the midheaven varies with latitude and
	// is real information.
	if len(c.Aspects) == 0 {
		t.Error("filtering removed every aspect")
	}
}
