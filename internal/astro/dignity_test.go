package astro

import "testing"

// The dignity tables are transcribed from tradition, so they are checked
// against the structural rules the schemes obey rather than trusted as typed.

// TestTermsAreWellFormed checks the Egyptian bounds. Each sign is divided into
// exactly five segments that tile the thirty degrees without gap or overlap,
// and the five rulers are the five non-luminary planets, each appearing once.
func TestTermsAreWellFormed(t *testing.T) {
	wanderers := []string{"mercury", "venus", "mars", "jupiter", "saturn"}

	for _, sign := range signs {
		bounds, ok := terms[sign.Name]
		if !ok {
			t.Errorf("%s has no bounds", sign.Name)
			continue
		}
		if len(bounds) != 5 {
			t.Errorf("%s has %d bounds, want 5", sign.Name, len(bounds))
			continue
		}

		// The segments must run from 0 to 30 end to end.
		if bounds[0].Start != 0 {
			t.Errorf("%s starts its bounds at %g, want 0", sign.Name, bounds[0].Start)
		}
		for i, b := range bounds {
			if b.End <= b.Start {
				t.Errorf("%s bound %d runs from %g to %g", sign.Name, i, b.Start, b.End)
			}
			if i > 0 && b.Start != bounds[i-1].End {
				t.Errorf("%s has a gap or overlap between %g and %g",
					sign.Name, bounds[i-1].End, b.Start)
			}
		}
		if last := bounds[len(bounds)-1]; last.End != 30 {
			t.Errorf("%s ends its bounds at %g, want 30", sign.Name, last.End)
		}

		// Each of the five planets rules exactly one segment. The Sun and
		// Moon never rule bounds.
		seen := map[string]int{}
		for _, b := range bounds {
			seen[b.Ruler]++
		}
		for _, planet := range wanderers {
			if seen[planet] != 1 {
				t.Errorf("%s gives %s %d bounds, want exactly 1", sign.Name, planet, seen[planet])
			}
		}
		if len(seen) != 5 {
			t.Errorf("%s has bounds ruled by %d different planets, want 5", sign.Name, len(seen))
		}
	}
}

// TestEveryDegreeHasATermRuler checks that no degree of the zodiac falls
// through the bounds.
func TestEveryDegreeHasATermRuler(t *testing.T) {
	for lon := 0.0; lon < 360; lon += 0.25 {
		sign := SignAt(lon)
		if ruler := termRulerAt(sign.Name, DegreeInSign(lon)); ruler == "" {
			t.Fatalf("no bound covers %.2f degrees, in %s", lon, sign.Name)
		}
	}
}

// TestFacesFollowTheChaldeanOrder checks the decans. There are thirty six of
// them, they run in the Chaldean sequence without restarting at a sign
// boundary, and the sequence has a period of seven, so the pattern only
// repeats after the whole zodiac.
func TestFacesFollowTheChaldeanOrder(t *testing.T) {
	// The first decan of Aries is ruled by Mars, and each following decan
	// takes the next planet in the sequence.
	want := 0
	for signIndex := 0; signIndex < 12; signIndex++ {
		for decan := 0; decan < 3; decan++ {
			degree := float64(decan)*10 + 5
			got := faceRulerAt(signIndex, degree)
			if got != chaldeanOrder[want%7] {
				t.Errorf("%s decan %d is ruled by %s, want %s",
					signs[signIndex].Name, decan+1, got, chaldeanOrder[want%7])
			}
			want++
		}
	}

	// A few readings that are quoted in every textbook.
	cases := []struct {
		sign   int
		degree float64
		want   string
	}{
		{0, 5, "mars"},   // Aries 1st
		{0, 15, "sun"},   // Aries 2nd
		{0, 25, "venus"}, // Aries 3rd
		{2, 25, "sun"},   // Gemini 3rd
		{11, 25, "mars"}, // Pisces 3rd, the last of the thirty six
		{4, 5, "saturn"}, // Leo 1st
	}
	for _, tc := range cases {
		if got := faceRulerAt(tc.sign, tc.degree); got != tc.want {
			t.Errorf("%s at %g is ruled by %s, want %s",
				signs[tc.sign].Name, tc.degree, got, tc.want)
		}
	}
}

// TestExaltationsAndFalls checks that each classical planet is exalted in one
// sign and falls in the sign opposite, and that no two share a sign.
func TestExaltationsAndFalls(t *testing.T) {
	used := map[string]string{}

	for planet, ex := range exaltations {
		if !classicalPlanets[planet] {
			t.Errorf("%s is not a classical planet but has an exaltation", planet)
		}
		if prev, dup := used[ex.Sign]; dup {
			t.Errorf("%s and %s are both exalted in %s", prev, planet, ex.Sign)
		}
		used[ex.Sign] = planet

		if ex.Degree < 0 || ex.Degree >= 30 {
			t.Errorf("%s is exalted at %g degrees, outside a sign", planet, ex.Degree)
		}

		// Exaltation and fall must be opposite, and the placement must
		// report them that way.
		var exSign, fallSign Sign
		for _, s := range signs {
			if s.Name == ex.Sign {
				exSign = s
			}
		}
		fallSign = signs[(exSign.Index+6)%12]

		if d := DignityOf(planet, float64(exSign.Index)*30+ex.Degree, SectDay); d == nil || !d.Exaltation {
			t.Errorf("%s is not reported as exalted in %s", planet, ex.Sign)
		}
		if d := DignityOf(planet, float64(fallSign.Index)*30+15, SectDay); d == nil || !d.Fall {
			t.Errorf("%s is not reported as in fall in %s", planet, fallSign.Name)
		}
	}

	if len(exaltations) != 7 {
		t.Errorf("there are %d exaltations, want one for each classical planet", len(exaltations))
	}
}

// TestDomicileAndDetriment checks that every classical planet rules some sign
// and is in detriment opposite it.
func TestDomicileAndDetriment(t *testing.T) {
	for planet := range classicalPlanets {
		var ruled []Sign
		for _, s := range signs {
			if s.Ruler == planet {
				ruled = append(ruled, s)
			}
		}
		if len(ruled) == 0 {
			t.Errorf("%s rules no sign", planet)
			continue
		}
		// The luminaries rule one sign each and the other five rule two.
		wantCount := 2
		if planet == "sun" || planet == "moon" {
			wantCount = 1
		}
		if len(ruled) != wantCount {
			t.Errorf("%s rules %d signs, want %d", planet, len(ruled), wantCount)
		}

		for _, s := range ruled {
			mid := float64(s.Index)*30 + 15
			if d := DignityOf(planet, mid, SectDay); d == nil || !d.Domicile {
				t.Errorf("%s is not reported as in domicile in %s", planet, s.Name)
			}
			opposite := signs[(s.Index+6)%12]
			oppositeMid := float64(opposite.Index)*30 + 15
			if d := DignityOf(planet, oppositeMid, SectDay); d == nil || !d.Detriment {
				t.Errorf("%s is not reported as in detriment in %s", planet, opposite.Name)
			}
		}
	}
}

// TestTriplicityFollowsSect checks that the day and night rulers swap with the
// sect and that the participating ruler holds in both.
func TestTriplicityFollowsSect(t *testing.T) {
	for element, rulers := range triplicityRulers {
		var sign Sign
		for _, s := range signs {
			if s.Element == element {
				sign = s
				break
			}
		}
		mid := float64(sign.Index)*30 + 15

		day, night, participating := rulers[0], rulers[1], rulers[2]

		if d := DignityOf(day, mid, SectDay); d == nil || !d.Triplicity {
			t.Errorf("%s should hold the %s triplicity by day", day, element)
		}
		if d := DignityOf(day, mid, SectNight); d != nil && d.Triplicity && day != participating {
			t.Errorf("%s should not hold the %s triplicity by night", day, element)
		}
		if d := DignityOf(night, mid, SectNight); d == nil || !d.Triplicity {
			t.Errorf("%s should hold the %s triplicity by night", night, element)
		}
		for _, sect := range []Sect{SectDay, SectNight} {
			if d := DignityOf(participating, mid, sect); d == nil || !d.Triplicity {
				t.Errorf("%s participates in the %s triplicity and should hold it by %s",
					participating, element, sect)
			}
		}
	}
}

// TestDignityOnlyForClassicalPlanets checks that nothing is invented for the
// bodies the scheme predates.
func TestDignityOnlyForClassicalPlanets(t *testing.T) {
	for _, body := range []string{
		"uranus", "neptune", "pluto", "chiron", "true_node", "mean_node",
		"south_node", "lilith", "ceres", "pallas", "juno", "vesta", "earth",
	} {
		if d := DignityOf(body, 100, SectDay); d != nil {
			t.Errorf("%s was given a dignity, but the scheme does not cover it", body)
		}
	}
}

func TestSectFromSunHouse(t *testing.T) {
	// Houses seven to twelve are above the horizon in every house system.
	for house := 1; house <= 6; house++ {
		if got := SectOf(house); got != SectNight {
			t.Errorf("the Sun in house %d gives %q, want night", house, got)
		}
	}
	for house := 7; house <= 12; house++ {
		if got := SectOf(house); got != SectDay {
			t.Errorf("the Sun in house %d gives %q, want day", house, got)
		}
	}
}
