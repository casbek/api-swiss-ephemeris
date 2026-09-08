package astro

import (
	"context"
	"math"
	"testing"

	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// TestVargaCatalogueIsConsistent checks the table itself.
func TestVargaCatalogueIsConsistent(t *testing.T) {
	list := Vargas()
	if len(list) != 16 {
		t.Errorf("got %d divisional charts, want the sixteen Parashara defines", len(list))
	}

	seen := map[string]bool{}
	for _, v := range list {
		if seen[v.Name] {
			t.Errorf("duplicate divisional chart %q", v.Name)
		}
		seen[v.Name] = true

		if v.Divisions < 1 {
			t.Errorf("%s divides the sign into %d parts", v.Name, v.Divisions)
		}
		if v.Label == "" || v.Purpose == "" {
			t.Errorf("%s is missing its label or purpose", v.Name)
		}
		if _, ok := LookupVarga(v.Name); !ok {
			t.Errorf("%s is listed but cannot be looked up", v.Name)
		}
	}

	for _, name := range DefaultVargas {
		if !seen[name] {
			t.Errorf("the default set names %q, which is not in the catalogue", name)
		}
	}
}

// TestEveryVargaMapsTheWholeCircle checks that no longitude falls through any
// division, and that every one lands on a real sign.
func TestEveryVargaMapsTheWholeCircle(t *testing.T) {
	for _, v := range Vargas() {
		for lon := 0.0; lon < 360; lon += 0.13 {
			index := VargaSignOf(v, lon)
			if index < 0 || index > 11 {
				t.Fatalf("%s maps %.2f to sign index %d", v.Name, lon, index)
			}

			division := divisionOf(v, lon)
			if division < 1 || division > v.Divisions {
				t.Fatalf("%s puts %.2f in division %d of %d",
					v.Name, lon, division, v.Divisions)
			}
		}
	}
}

// TestNavamsaMatchesTheContinuousDivision is the strongest check available for
// a varga. The navamsa is the one division that runs unbroken round the whole
// zodiac: a ninth of a sign is 3°20', and the sign a point lands in is simply
// how many of those it stands past the start of Aries. Deriving it that way and
// through the movable, fixed and dual rules must give the same answer at every
// degree, and it only does if all three starting rules are right.
func TestNavamsaMatchesTheContinuousDivision(t *testing.T) {
	v, ok := LookupVarga("d9")
	if !ok {
		t.Fatal("the navamsa is missing")
	}

	const ninth = 30.0 / 9.0
	for lon := 0.0; lon < 360; lon += 0.07 {
		want := int(lon/ninth) % 12
		if got := VargaSignOf(v, lon); got != want {
			t.Fatalf("the navamsa of %.4f is sign %d, but counting ninths from Aries gives %d",
				lon, got, want)
		}
	}
}

// TestVargaStartingRules checks a placement in each division against how the
// rule is stated, at the first degree of a sign of each kind.
func TestVargaStartingRules(t *testing.T) {
	cases := []struct {
		varga string
		// longitude has to fall inside the first part of the division for the
		// answer to be the starting sign itself. A fortieth of a sign is only
		// 45 minutes wide, so for the finest divisions a whole degree is
		// already the second part.
		longitude float64
		want      int
		why       string
	}{
		// The rashi chart is no division: every point stays where it is.
		{"d1", 0, aries, "Aries stays Aries"},
		{"d1", 100, cancer, "10 Cancer stays in Cancer"},

		// The hora gives the first half of an odd sign to the Sun and of an
		// even sign to the Moon.
		{"d2", 5, leo, "the first half of Aries is the hora of the Sun"},
		{"d2", 20, cancer, "the second half of Aries is the hora of the Moon"},
		{"d2", 35, cancer, "the first half of Taurus is the hora of the Moon"},
		{"d2", 50, leo, "the second half of Taurus is the hora of the Sun"},

		// The drekkana gives each third to the sign, the fifth and the ninth.
		{"d3", 5, aries, "the first third of Aries"},
		{"d3", 15, leo, "the second third of Aries is the fifth from it"},
		{"d3", 25, sagittarius, "the last third of Aries is the ninth from it"},

		// The chaturthamsha walks the angles.
		{"d4", 3, aries, "the first quarter of Aries"},
		{"d4", 10, cancer, "the second quarter is the fourth from it"},

		// The saptamsha starts an odd sign from itself and an even one from
		// the seventh.
		{"d7", 1, aries, "the first seventh of Aries"},
		{"d7", 31, scorpio, "the first seventh of Taurus is the seventh from it"},

		// The navamsa: movable from itself, fixed from the ninth, dual from
		// the fifth.
		{"d9", 1, aries, "the first navamsa of movable Aries"},
		{"d9", 31, capricorn, "the first navamsa of fixed Taurus is the ninth from it"},
		{"d9", 61, libra, "the first navamsa of dual Gemini is the fifth from it"},

		// The dashamsha starts an odd sign from itself and an even one from
		// the ninth.
		{"d10", 1, aries, "the first tenth of Aries"},
		{"d10", 31, capricorn, "the first tenth of Taurus is the ninth from it"},

		// The dwadashamsha starts from the sign and walks one at a time.
		{"d12", 1, aries, "the first twelfth of Aries"},
		{"d12", 4, taurus, "the second twelfth of Aries"},

		// These count from a fixed sign whatever sign the body is in.
		{"d16", 1, aries, "movable signs start the shodashamsha at Aries"},
		{"d16", 31, leo, "fixed signs start it at Leo"},
		{"d16", 61, sagittarius, "dual signs start it at Sagittarius"},
		{"d20", 31, sagittarius, "fixed signs start the vimshamsha at Sagittarius"},
		{"d24", 1, leo, "odd signs start the chaturvimshamsha at Leo"},
		{"d24", 31, cancer, "even signs start it at Cancer"},
		{"d27", 1, aries, "fire signs start the bhamsha at Aries"},
		{"d27", 31, cancer, "earth signs start it at Cancer"},
		{"d40", 0.3, aries, "odd signs start the khavedamsha at Aries"},
		{"d40", 30.3, libra, "even signs start it at Libra"},
		{"d45", 60.3, sagittarius, "dual signs start the akshavedamsha at Sagittarius"},

		// The trimshamsha gives five unequal stretches to the five planets
		// that are neither luminary.
		{"d30", 2, aries, "the first five degrees of an odd sign go to Mars"},
		{"d30", 7, aquarius, "the next five to Saturn"},
		{"d30", 15, sagittarius, "the next eight to Jupiter"},
		{"d30", 20, gemini, "the next seven to Mercury"},
		{"d30", 28, libra, "the last five to Venus"},
		{"d30", 32, taurus, "the first five degrees of an even sign go to Venus"},
		{"d30", 58, scorpio, "the last five of an even sign go to Mars"},

		// The shashtiamsha walks one sign per half degree from the sign
		// itself.
		{"d60", 0.2, aries, "the first shashtiamsha of Aries"},
		{"d60", 0.7, taurus, "the second"},
	}

	for _, tc := range cases {
		v, ok := LookupVarga(tc.varga)
		if !ok {
			t.Errorf("%s is missing from the catalogue", tc.varga)
			continue
		}
		got := VargaSignOf(v, tc.longitude)
		if got != tc.want {
			t.Errorf("%s of %.2f is %s, want %s (%s)",
				tc.varga, tc.longitude, signs[got].Name, signs[tc.want].Name, tc.why)
		}
	}
}

// TestTrimshamshaCoversTheSign checks the one division with unequal parts. The
// five stretches must tile the thirty degrees, and the five planets that are
// neither luminary must each take exactly one.
func TestTrimshamshaCoversTheSign(t *testing.T) {
	for name, table := range map[string][]struct {
		Upto float64
		Sign int
	}{"odd": trimshamshaOdd, "even": trimshamshaEven} {

		if len(table) != 5 {
			t.Errorf("the %s trimshamsha has %d stretches, want 5", name, len(table))
			continue
		}
		if table[len(table)-1].Upto != 30 {
			t.Errorf("the %s trimshamsha ends at %g, want 30", name, table[len(table)-1].Upto)
		}

		prev := 0.0
		seen := map[string]bool{}
		for i, part := range table {
			if part.Upto <= prev {
				t.Errorf("the %s trimshamsha stretch %d runs from %g to %g",
					name, i, prev, part.Upto)
			}
			prev = part.Upto

			// Each stretch belongs to a planet, named by the sign it maps to.
			ruler := signs[part.Sign].Ruler
			if ruler == "sun" || ruler == "moon" {
				t.Errorf("the %s trimshamsha gives a stretch to the %s, but the "+
					"luminaries take no part in it", name, ruler)
			}
			if seen[ruler] {
				t.Errorf("the %s trimshamsha gives %s two stretches", name, ruler)
			}
			seen[ruler] = true
		}
		if len(seen) != 5 {
			t.Errorf("the %s trimshamsha is shared among %d planets, want 5", name, len(seen))
		}
	}
}

// TestHoraOnlyUsesTheLuminaries checks that the two-part division maps nowhere
// but Cancer and Leo, the signs of the Moon and the Sun.
func TestHoraOnlyUsesTheLuminaries(t *testing.T) {
	v, _ := LookupVarga("d2")

	seen := map[int]int{}
	for lon := 0.0; lon < 360; lon += 0.5 {
		index := VargaSignOf(v, lon)
		if index != cancer && index != leo {
			t.Fatalf("the hora of %.1f is %s, but it divides only between Cancer and Leo",
				lon, signs[index].Name)
		}
		seen[index]++
	}
	// Half the circle goes to each.
	if seen[cancer] != seen[leo] {
		t.Errorf("the hora gives Cancer %d samples and Leo %d, want an even split",
			seen[cancer], seen[leo])
	}
}

// TestDivisionalsPlaceEveryBody checks the endpoint layer.
func TestDivisionalsPlaceEveryBody(t *testing.T) {
	e := newTestEngine(t)

	result, err := e.Divisionals(context.Background(), VargaRequest{
		Natal:  referenceRequest,
		Charts: []string{"d1", "d9", "d10"},
	})
	if err != nil {
		t.Fatalf("Divisionals failed: %v", err)
	}
	if len(result.Charts) != 3 {
		t.Fatalf("got %d charts, want 3", len(result.Charts))
	}

	// Asking for a Vedic technique gets the sidereal zodiac and whole sign
	// houses, whether or not the request said so.
	if result.Natal.Settings.Zodiac != ZodiacSidereal {
		t.Errorf("the chart was cast in the %s zodiac", result.Natal.Settings.Zodiac)
	}
	if result.Natal.Settings.HouseSystem.Name != "whole_sign" {
		t.Errorf("houses were divided by %s, want whole sign",
			result.Natal.Settings.HouseSystem.Name)
	}

	rashi := result.Charts[0]
	if rashi.Name != "d1" {
		t.Fatalf("the first chart is %s, want d1", rashi.Name)
	}
	if len(rashi.Bodies) != len(result.Natal.Positions) {
		t.Errorf("the rashi chart holds %d bodies but the natal chart has %d",
			len(rashi.Bodies), len(result.Natal.Positions))
	}
	if rashi.Ascendant == nil {
		t.Error("the rashi chart has no ascendant, although the birth time is known")
	}

	// The rashi chart is no division at all, so every body must sit in the
	// sign it already occupies.
	for _, b := range rashi.Bodies {
		var natal Position
		for _, p := range result.Natal.Positions {
			if p.Body == b.Body {
				natal = p
			}
		}
		if b.SignIndex != natal.SignIndex {
			t.Errorf("the rashi chart puts %s in %s, but it is in %s",
				b.Body, b.Sign, natal.Sign)
		}
		if math.Abs(b.RashiLongitude-natal.Longitude) > 1e-9 {
			t.Errorf("%s reports longitude %.6f, want %.6f",
				b.Body, b.RashiLongitude, natal.Longitude)
		}
	}

	// Every placement must agree with the mapping applied on its own.
	for _, chart := range result.Charts {
		v, _ := LookupVarga(chart.Name)
		for _, b := range chart.Bodies {
			if want := VargaSignOf(v, b.RashiLongitude); b.SignIndex != want {
				t.Errorf("%s puts %s in %s, but its longitude %.4f maps to %s",
					chart.Name, b.Body, b.Sign, b.RashiLongitude, signs[want].Name)
			}
		}
	}
}

func TestDivisionalsRejectUnknownChart(t *testing.T) {
	e := newTestEngine(t)

	_, err := e.Divisionals(context.Background(), VargaRequest{
		Natal:  referenceRequest,
		Charts: []string{"d9", "d13"},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	fe, ok := err.(*tz.FieldError)
	if !ok {
		t.Fatalf("error is %T, want a *tz.FieldError", err)
	}
	if fe.Field != "charts" {
		t.Errorf("field = %q, want charts", fe.Field)
	}
}

func TestDivisionalsDefaultToRashiAndNavamsa(t *testing.T) {
	e := newTestEngine(t)

	result, err := e.Divisionals(context.Background(), VargaRequest{Natal: referenceRequest})
	if err != nil {
		t.Fatalf("Divisionals failed: %v", err)
	}
	if len(result.Charts) != 2 ||
		result.Charts[0].Name != "d1" || result.Charts[1].Name != "d9" {
		t.Errorf("the default set is %v, want the rashi chart and the navamsa",
			result.Charts)
	}
}
