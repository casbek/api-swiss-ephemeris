package astro

// Essential dignity: how well placed a planet is by sign, degree and sect.
//
// Only the seven classical planets have essential dignity. The scheme predates
// the discovery of the outer planets, and the assignments given to them since
// are modern additions that practitioners disagree about, so nothing is
// reported for them rather than picking a side.

// Ptolemaic weights. A planet in its own sign is the strongest placement and
// the one opposite its exaltation the weakest.
const (
	ScoreDomicile   = 5
	ScoreExaltation = 4
	ScoreTriplicity = 3
	ScoreTerm       = 2
	ScoreFace       = 1
	ScoreDetriment  = -5
	ScoreFall       = -4
)

// classicalPlanets are the only bodies essential dignity applies to.
var classicalPlanets = map[string]bool{
	"sun": true, "moon": true, "mercury": true, "venus": true,
	"mars": true, "jupiter": true, "saturn": true,
}

// exaltations maps a planet to the sign and degree of its exaltation.
var exaltations = map[string]struct {
	Sign   string
	Degree float64
}{
	"sun":     {"aries", 19},
	"moon":    {"taurus", 3},
	"mercury": {"virgo", 15},
	"venus":   {"pisces", 27},
	"mars":    {"capricorn", 28},
	"jupiter": {"cancer", 15},
	"saturn":  {"libra", 21},
}

// triplicityRulers gives the Dorothean rulers of each element: the first rules
// by day, the second by night, the third participates in both.
var triplicityRulers = map[string][3]string{
	"fire":  {"sun", "jupiter", "saturn"},
	"earth": {"venus", "moon", "mars"},
	"air":   {"saturn", "mercury", "jupiter"},
	"water": {"venus", "mars", "moon"},
}

// bound is one segment of a sign ruled by a planet.
type bound struct {
	Ruler string
	Start float64
	End   float64
}

// terms are the Egyptian bounds, the division most commonly used. Each sign is
// split into five unequal segments, one for each non-luminary planet.
var terms = map[string][]bound{
	"aries":       {{"jupiter", 0, 6}, {"venus", 6, 12}, {"mercury", 12, 20}, {"mars", 20, 25}, {"saturn", 25, 30}},
	"taurus":      {{"venus", 0, 8}, {"mercury", 8, 14}, {"jupiter", 14, 22}, {"saturn", 22, 27}, {"mars", 27, 30}},
	"gemini":      {{"mercury", 0, 6}, {"jupiter", 6, 12}, {"venus", 12, 17}, {"mars", 17, 24}, {"saturn", 24, 30}},
	"cancer":      {{"mars", 0, 7}, {"venus", 7, 13}, {"mercury", 13, 19}, {"jupiter", 19, 26}, {"saturn", 26, 30}},
	"leo":         {{"jupiter", 0, 6}, {"venus", 6, 11}, {"saturn", 11, 18}, {"mercury", 18, 24}, {"mars", 24, 30}},
	"virgo":       {{"mercury", 0, 7}, {"venus", 7, 17}, {"jupiter", 17, 21}, {"mars", 21, 28}, {"saturn", 28, 30}},
	"libra":       {{"saturn", 0, 6}, {"mercury", 6, 14}, {"jupiter", 14, 21}, {"venus", 21, 28}, {"mars", 28, 30}},
	"scorpio":     {{"mars", 0, 7}, {"venus", 7, 11}, {"mercury", 11, 19}, {"jupiter", 19, 24}, {"saturn", 24, 30}},
	"sagittarius": {{"jupiter", 0, 12}, {"venus", 12, 17}, {"mercury", 17, 21}, {"saturn", 21, 26}, {"mars", 26, 30}},
	"capricorn":   {{"mercury", 0, 7}, {"jupiter", 7, 14}, {"venus", 14, 22}, {"saturn", 22, 26}, {"mars", 26, 30}},
	"aquarius":    {{"mercury", 0, 7}, {"venus", 7, 13}, {"jupiter", 13, 20}, {"mars", 20, 25}, {"saturn", 25, 30}},
	"pisces":      {{"venus", 0, 12}, {"jupiter", 12, 16}, {"mercury", 16, 19}, {"mars", 19, 28}, {"saturn", 28, 30}},
}

// chaldeanOrder is the sequence the faces run through, from the slowest
// visible planet to the fastest and back. Each sign holds three faces of ten
// degrees, and the sequence continues unbroken from one sign to the next.
var chaldeanOrder = [7]string{"mars", "sun", "venus", "mercury", "moon", "saturn", "jupiter"}

// Dignity is the essential dignity of one placement.
type Dignity struct {
	// Domicile is true when the planet is in the sign it rules.
	Domicile bool `json:"domicile,omitempty"`
	// Exaltation is true when the planet is in its sign of exaltation.
	Exaltation bool `json:"exaltation,omitempty"`
	// Detriment is true when the planet is opposite the sign it rules.
	Detriment bool `json:"detriment,omitempty"`
	// Fall is true when the planet is opposite its exaltation.
	Fall bool `json:"fall,omitempty"`

	// Triplicity is true when the planet rules the element by the sect of
	// the chart, or participates in it.
	Triplicity bool `json:"triplicity,omitempty"`

	// TermRuler and FaceRuler name the planets ruling the bound and the
	// decan the placement falls in.
	TermRuler string `json:"term_ruler"`
	FaceRuler string `json:"face_ruler"`

	// InOwnTerm and InOwnFace are true when the planet rules its own bound
	// or decan.
	InOwnTerm bool `json:"in_own_term,omitempty"`
	InOwnFace bool `json:"in_own_face,omitempty"`

	// Score sums the Ptolemaic weights above. It is a summary, not a verdict:
	// practitioners weigh these differently.
	Score int `json:"score"`
}

// Sect is whether a chart was cast by day or by night. It decides which
// triplicity ruler applies, among much else in traditional practice.
type Sect string

const (
	SectDay   Sect = "day"
	SectNight Sect = "night"
)

// DignityOf reports the essential dignity of a body at a longitude.
//
// sect selects the triplicity ruler. It returns nil for anything but the seven
// classical planets.
func DignityOf(body string, longitude float64, sect Sect) *Dignity {
	if !classicalPlanets[body] {
		return nil
	}

	sign := SignAt(longitude)
	degree := DegreeInSign(longitude)
	opposite := signs[(sign.Index+6)%12]

	d := &Dignity{}

	// Rulership is read from the sign table, so the two can never disagree.
	if sign.Ruler == body {
		d.Domicile = true
		d.Score += ScoreDomicile
	}
	if opposite.Ruler == body {
		d.Detriment = true
		d.Score += ScoreDetriment
	}

	if ex, ok := exaltations[body]; ok {
		if ex.Sign == sign.Name {
			d.Exaltation = true
			d.Score += ScoreExaltation
		}
		if ex.Sign == opposite.Name {
			d.Fall = true
			d.Score += ScoreFall
		}
	}

	if rulers, ok := triplicityRulers[sign.Element]; ok {
		primary := rulers[0]
		if sect == SectNight {
			primary = rulers[1]
		}
		// The participating ruler counts in both sects.
		if body == primary || body == rulers[2] {
			d.Triplicity = true
			d.Score += ScoreTriplicity
		}
	}

	d.TermRuler = termRulerAt(sign.Name, degree)
	if d.TermRuler == body {
		d.InOwnTerm = true
		d.Score += ScoreTerm
	}

	d.FaceRuler = faceRulerAt(sign.Index, degree)
	if d.FaceRuler == body {
		d.InOwnFace = true
		d.Score += ScoreFace
	}

	return d
}

// termRulerAt returns the planet ruling the bound a degree falls in.
func termRulerAt(sign string, degree float64) string {
	for _, b := range terms[sign] {
		if degree >= b.Start && degree < b.End {
			return b.Ruler
		}
	}
	// Only a degree of exactly 30 could reach here, which DegreeInSign
	// cannot produce, but the last bound is the right answer if it did.
	if bounds := terms[sign]; len(bounds) > 0 {
		return bounds[len(bounds)-1].Ruler
	}
	return ""
}

// faceRulerAt returns the planet ruling the decan a degree falls in. The
// Chaldean sequence runs continuously across sign boundaries, so the face is
// found from its position in the whole zodiac rather than within the sign.
func faceRulerAt(signIndex int, degree float64) string {
	decan := int(degree / 10)
	if decan > 2 {
		decan = 2
	}
	return chaldeanOrder[(signIndex*3+decan)%7]
}

// SectOf reports whether a chart is a day or a night chart.
//
// The Sun above the horizon makes it a day chart. Houses seven to twelve are
// the ones above the horizon, which holds for every house system, so the Sun's
// house is the reliable test.
func SectOf(sunHouse int) Sect {
	if sunHouse >= 7 && sunHouse <= 12 {
		return SectDay
	}
	return SectNight
}
