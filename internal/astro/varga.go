package astro

import (
	"context"
	"fmt"
	"strings"

	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// A divisional chart, or varga, cuts each sign into equal parts and reads each
// part as a sign of its own. The sixteen Parashara defines run from the rashi
// chart itself, which is no division at all, down to the shashtiamsha of half a
// degree.
//
// What differs between them is only where the count starts. That is set by the
// sign's nature: whether it is odd or even, movable, fixed or dual, or which
// element it belongs to. Holding those rules as data rather than as code means
// each one reads as a single line that can be checked against a text.

// startRule says where the count begins for a division.
type startRule int

const (
	// startFromSign counts from the sign the position is in.
	startFromSign startRule = iota
	// startOddEven counts from one sign for odd signs and another for even.
	startOddEven
	// startByQuality counts from one sign for movable, another for fixed and
	// a third for dual.
	startByQuality
	// startByElement counts from a different sign for each element.
	startByElement
	// startHora is the two-part division, which maps to just two signs and
	// so follows no offset rule.
	startHora
	// startTrimshamsha is the thirty-part division, which is the one varga
	// with unequal parts.
	startTrimshamsha
)

// Varga is one divisional chart.
type Varga struct {
	// Name is the D-number the chart is known by, such as d9.
	Name  string `json:"name"`
	Label string `json:"label"`

	// Divisions is how many parts each sign is cut into.
	Divisions int `json:"divisions"`

	// Purpose is the matter the division is traditionally read for. It names
	// the subject rather than interpreting it, the way a house number does.
	Purpose string `json:"purpose"`

	rule startRule

	// relative says how to read the starting values below. Some divisions
	// begin counting from the sign the body is in, so their values are
	// offsets from it; others begin from a fixed sign whatever the body is
	// in, so their values name that sign. The navamsa and the shodashamsha
	// use the same rule with opposite senses, which is exactly the confusion
	// this flag exists to prevent.
	relative bool

	// step is how far the count moves per part, for the divisions that walk
	// forward by more than one sign. Zero means one.
	step int

	// odd, even are the starting signs for startOddEven.
	odd, even int

	// movable, fixed, dual are the starting signs for startByQuality.
	movable, fixed, dual int

	// fire, earth, air, water are the starting signs for startByElement.
	fire, earth, air, water int
}

// Sign indices, for the tables below.
const (
	aries       = 0
	taurus      = 1
	gemini      = 2
	cancer      = 3
	leo         = 4
	virgo       = 5
	libra       = 6
	scorpio     = 7
	sagittarius = 8
	capricorn   = 9
	aquarius    = 10
	pisces      = 11
)

// vargas are the sixteen divisional charts, in the order they are always
// listed.
var vargas = []Varga{
	{Name: "d1", Label: "Rashi", Divisions: 1, Purpose: "the body and the life as a whole",
		rule: startFromSign},

	{Name: "d2", Label: "Hora", Divisions: 2, Purpose: "wealth",
		rule: startHora},

	// Each third goes to the sign itself, the fifth from it and the ninth,
	// which is a step of four signs.
	{Name: "d3", Label: "Drekkana", Divisions: 3, Purpose: "siblings",
		rule: startFromSign, step: 4},

	// Each quarter goes to the sign, the fourth, the seventh and the tenth:
	// the four angles, a step of three.
	{Name: "d4", Label: "Chaturthamsha", Divisions: 4, Purpose: "property and fortune",
		rule: startFromSign, step: 3},

	// An odd sign counts from itself, an even one from the seventh from it.
	{Name: "d7", Label: "Saptamsha", Divisions: 7, Purpose: "children",
		rule: startOddEven, relative: true, odd: 0, even: 6},

	// A movable sign counts from itself, a fixed one from the ninth from it
	// and a dual one from the fifth, which together make the division run
	// unbroken round the whole zodiac.
	{Name: "d9", Label: "Navamsa", Divisions: 9, Purpose: "marriage, and the chart's underlying strength",
		rule: startByQuality, relative: true, movable: 0, fixed: 8, dual: 4},

	// An odd sign counts from itself, an even one from the ninth from it.
	{Name: "d10", Label: "Dashamsha", Divisions: 10, Purpose: "action and career",
		rule: startOddEven, relative: true, odd: 0, even: 8},

	{Name: "d12", Label: "Dwadashamsha", Divisions: 12, Purpose: "parents",
		rule: startFromSign},

	{Name: "d16", Label: "Shodashamsha", Divisions: 16, Purpose: "vehicles and comforts",
		rule: startByQuality, movable: aries, fixed: leo, dual: sagittarius},

	{Name: "d20", Label: "Vimshamsha", Divisions: 20, Purpose: "religious practice",
		rule: startByQuality, movable: aries, fixed: sagittarius, dual: leo},

	{Name: "d24", Label: "Chaturvimshamsha", Divisions: 24, Purpose: "learning",
		rule: startOddEven, odd: leo, even: cancer},

	{Name: "d27", Label: "Bhamsha", Divisions: 27, Purpose: "strength and weakness",
		rule: startByElement, fire: aries, earth: cancer, air: libra, water: capricorn},

	{Name: "d30", Label: "Trimshamsha", Divisions: 30, Purpose: "misfortune",
		rule: startTrimshamsha},

	{Name: "d40", Label: "Khavedamsha", Divisions: 40, Purpose: "auspicious and inauspicious effects",
		rule: startOddEven, odd: aries, even: libra},

	{Name: "d45", Label: "Akshavedamsha", Divisions: 45, Purpose: "general conduct",
		rule: startByQuality, movable: aries, fixed: leo, dual: sagittarius},

	{Name: "d60", Label: "Shashtiamsha", Divisions: 60, Purpose: "all matters, and past deeds",
		rule: startFromSign},
}

var vargaIndex = func() map[string]Varga {
	m := make(map[string]Varga, len(vargas))
	for _, v := range vargas {
		m[v.Name] = v
	}
	return m
}()

// Vargas returns the sixteen divisional charts.
func Vargas() []Varga { return append([]Varga(nil), vargas...) }

// LookupVarga finds a divisional chart by its D-number.
func LookupVarga(name string) (Varga, bool) {
	v, ok := vargaIndex[strings.ToLower(strings.TrimSpace(name))]
	return v, ok
}

// DefaultVargas are the ones returned when a request names none: the rashi
// chart and the navamsa, which no Vedic reading is done without.
var DefaultVargas = []string{"d1", "d9"}

// horaSigns is the two-part division. It maps to Leo and Cancer alone, the
// signs of the Sun and the Moon, rather than counting round the zodiac: an odd
// sign gives its first half to the Sun and its second to the Moon, and an even
// sign the other way about.
var horaSigns = [2][2]int{
	{leo, cancer}, // odd signs
	{cancer, leo}, // even signs
}

// trimshamshaOdd is the thirty-part division for an odd sign: five unequal
// stretches given to the five planets that are neither luminary, in the order
// Mars, Saturn, Jupiter, Mercury, Venus.
var trimshamshaOdd = []struct {
	Upto float64
	Sign int
}{
	{5, aries},        // Mars
	{10, aquarius},    // Saturn
	{18, sagittarius}, // Jupiter
	{25, gemini},      // Mercury
	{30, libra},       // Venus
}

// trimshamshaEven is the same for an even sign, reversed in order and given to
// the other sign of each planet.
var trimshamshaEven = []struct {
	Upto float64
	Sign int
}{
	{5, taurus},     // Venus
	{12, virgo},     // Mercury
	{20, pisces},    // Jupiter
	{25, capricorn}, // Saturn
	{30, scorpio},   // Mars
}

// VargaSignOf maps a longitude into a divisional chart, returning the index of
// the sign it falls in there.
func VargaSignOf(v Varga, longitude float64) int {
	lon := Normalize(longitude)
	sign := int(lon / 30)
	if sign > 11 {
		sign = 11
	}
	degree := lon - float64(sign)*30

	switch v.rule {
	case startHora:
		half := 0
		if degree >= 15 {
			half = 1
		}
		return horaSigns[sign%2][half]

	case startTrimshamsha:
		table := trimshamshaOdd
		if sign%2 == 1 {
			table = trimshamshaEven
		}
		for _, part := range table {
			if degree < part.Upto {
				return part.Sign
			}
		}
		return table[len(table)-1].Sign
	}

	part := int(degree / (30 / float64(v.Divisions)))
	if part >= v.Divisions {
		part = v.Divisions - 1
	}

	step := v.step
	if step == 0 {
		step = 1
	}

	var base int
	switch v.rule {
	case startFromSign:
		base = 0
	case startOddEven:
		// Aries is the first sign, so the even indices are the odd signs.
		if sign%2 == 0 {
			base = v.odd
		} else {
			base = v.even
		}
	case startByQuality:
		switch signs[sign].Modality {
		case "cardinal":
			base = v.movable
		case "fixed":
			base = v.fixed
		default:
			base = v.dual
		}
	case startByElement:
		switch signs[sign].Element {
		case "fire":
			base = v.fire
		case "earth":
			base = v.earth
		case "air":
			base = v.air
		default:
			base = v.water
		}
	}

	start := base
	if v.relative || v.rule == startFromSign {
		start = sign + base
	}
	return ((start+part*step)%12 + 12) % 12
}

// VargaPosition is where a body falls in a divisional chart.
type VargaPosition struct {
	Body  string `json:"body"`
	Label string `json:"label,omitempty"`

	Sign      string `json:"sign"`
	SignIndex int    `json:"sign_index"`

	// Division is which part of its rashi sign the body fell in, from 1 up.
	Division int `json:"division"`

	// RashiLongitude is where the body actually is, which the division was
	// read from. A divisional chart has no longitudes of its own: it places
	// bodies in signs and nothing finer.
	RashiLongitude float64 `json:"rashi_longitude"`
}

// VargaChart is one divisional chart.
type VargaChart struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Divisions int    `json:"divisions"`
	Purpose   string `json:"purpose"`

	// Ascendant is the rising sign of the division, when the birth time is
	// known.
	Ascendant *VargaPosition `json:"ascendant,omitempty"`

	Bodies []VargaPosition `json:"bodies"`
}

// VargaRequest asks for divisional charts of a birth.
type VargaRequest struct {
	Natal ChartRequest `json:"natal"`

	// Charts names the divisions wanted, such as d9. Defaults to the rashi
	// chart and the navamsa.
	Charts []string `json:"charts,omitempty"`
}

// VargaResult is the divisional charts, with the chart they came from.
type VargaResult struct {
	Natal  *Chart
	Charts []VargaChart
}

// Divisionals computes the requested divisional charts.
func (e *Engine) Divisionals(ctx context.Context, req VargaRequest) (*VargaResult, error) {
	names := req.Charts
	if len(names) == 0 {
		names = DefaultVargas
	}

	wanted := make([]Varga, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		v, ok := LookupVarga(name)
		if !ok {
			return nil, &tz.FieldError{
				Field: "charts",
				Message: fmt.Sprintf("%q is not a divisional chart; see /v1/reference/divisional-charts",
					name),
			}
		}
		if seen[v.Name] {
			continue
		}
		seen[v.Name] = true
		wanted = append(wanted, v)
	}

	// Divisional charts belong to the sidereal zodiac, like everything else
	// in this tradition. A request that does not say gets it.
	natalReq := req.Natal
	if natalReq.Settings.Zodiac == "" {
		natalReq.Settings.Zodiac = string(ZodiacSidereal)
	}
	// The rashi chart is read by whole signs, so that is the default division
	// of houses here too.
	if natalReq.Settings.HouseSystem == "" {
		natalReq.Settings.HouseSystem = "whole_sign"
	}

	natal, err := e.Cast(ctx, natalReq)
	if err != nil {
		return nil, prefixField(err, "natal")
	}

	result := &VargaResult{Natal: natal, Charts: make([]VargaChart, 0, len(wanted))}
	for _, v := range wanted {
		result.Charts = append(result.Charts, buildVarga(v, natal))
	}
	return result, nil
}

// buildVarga places every body of a chart into one division.
func buildVarga(v Varga, natal *Chart) VargaChart {
	chart := VargaChart{
		Name:      v.Name,
		Label:     v.Label,
		Divisions: v.Divisions,
		Purpose:   v.Purpose,
		Bodies:    make([]VargaPosition, 0, len(natal.Positions)),
	}

	for _, p := range natal.Positions {
		chart.Bodies = append(chart.Bodies, placeInVarga(v, p.Body, p.Label, p.Longitude))
	}

	// The rising degree divides like any other point, and the sign it lands
	// in is the ascendant of the division.
	if natal.Houses != nil && len(natal.Houses.Angles) > 0 {
		asc := natal.Houses.Angles[0]
		placed := placeInVarga(v, asc.Body, asc.Label, asc.Longitude)
		chart.Ascendant = &placed
	}
	return chart
}

// divisionOf reports which part of its sign a longitude falls in, counting
// from one.
//
// Two of the sixteen do not cut the sign into equal parts. The hora has two
// halves, and the trimshamsha five stretches of unequal width, so for those the
// answer is the part rather than a thirtieth of the sign.
func divisionOf(v Varga, longitude float64) int {
	degree := DegreeInSign(longitude)

	switch v.rule {
	case startHora:
		if degree >= 15 {
			return 2
		}
		return 1

	case startTrimshamsha:
		sign := int(Normalize(longitude) / 30)
		table := trimshamshaOdd
		if sign%2 == 1 {
			table = trimshamshaEven
		}
		for i, part := range table {
			if degree < part.Upto {
				return i + 1
			}
		}
		return len(table)
	}

	division := int(degree/(30/float64(v.Divisions))) + 1
	if division > v.Divisions {
		division = v.Divisions
	}
	return division
}

// placeInVarga maps one longitude into a division.
func placeInVarga(v Varga, body, label string, longitude float64) VargaPosition {
	lon := Normalize(longitude)
	index := VargaSignOf(v, lon)

	division := divisionOf(v, lon)

	return VargaPosition{
		Body:           body,
		Label:          label,
		Sign:           signs[index].Name,
		SignIndex:      index,
		Division:       division,
		RashiLongitude: lon,
	}
}
