package astro

import (
	"math"
	"sort"
	"time"
)

// lookaheadDays is the step used to tell an applying aspect from a separating
// one. It is short enough that even the Moon, the fastest body, moves only a
// thousandth of a degree, so the comparison stays meaningful right up to the
// moment an aspect perfects.
const lookaheadDays = 1e-4

// Aspect is one angular relationship found between two positions.
type Aspect struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`

	// Angle is the exact angle of the aspect, such as 120 for a trine.
	Angle float64 `json:"angle"`

	// Separation is the actual angle between the two positions.
	Separation float64 `json:"separation"`

	// Orb is how far the separation is from exact, always positive.
	Orb float64 `json:"orb"`

	// MaxOrb is the widest orb this pair was allowed for this aspect. The
	// ratio of Orb to MaxOrb is a fair measure of how tight the aspect is.
	MaxOrb float64 `json:"max_orb"`

	// IsApplying is true when the two bodies are moving towards the exact
	// angle, false when they are moving apart. An applying aspect is
	// conventionally read as the stronger of the two.
	IsApplying bool `json:"is_applying"`

	// IsExact is true when the orb is under a minute of arc.
	IsExact bool `json:"is_exact"`

	// ExactAt is when the aspect perfects, for a comparison where one side
	// moves and the other stands still, such as a transit. It is absent when
	// nothing moves, when the moving body is at a station, or when the
	// crossing is further off than the search allows.
	//
	// A body that turns retrograde over the aspect perfects it three times.
	// This reports the nearest of them.
	ExactAt *time.Time `json:"exact_at,omitempty"`
}

// exactThreshold is one minute of arc, in degrees.
const exactThreshold = 1.0 / 60.0

// tautologies are pairs of points that are opposite one another by
// construction. The descendant is defined as the degree opposite the
// ascendant, the imum coeli as the degree opposite the midheaven, and the
// south node as the point opposite the north. An opposition between any of
// them is exact in every chart ever cast, so reporting it would put a fact
// about the definitions at the top of every aspect list and say nothing about
// the chart.
var tautologies = map[string]string{
	"ascendant":  "descendant",
	"descendant": "ascendant",
	"midheaven":  "imum_coeli",
	"imum_coeli": "midheaven",
	"true_node":  "south_node",
	"south_node": "true_node",
	"mean_node":  "south_node",
}

// isTautologous reports whether two points are opposite by definition.
func isTautologous(a, b string) bool {
	return tautologies[a] == b || tautologies[b] == a
}

// FindAspects returns every aspect between the given positions.
//
// Each unordered pair is examined once. Where a pair satisfies more than one
// aspect, which can happen when orbs are widened, only the tightest is kept:
// reporting a body as both conjunct and semisextile the same point would be
// noise rather than information.
func FindAspects(positions []Position, types []AspectType, orbs map[string]float64) []Aspect {
	var found []Aspect

	for i := 0; i < len(positions); i++ {
		for j := i + 1; j < len(positions); j++ {
			a, b := positions[i], positions[j]
			if isTautologous(a.Body, b.Body) {
				continue
			}

			var best *Aspect
			for _, t := range types {
				hit, ok := aspectBetween(a, b, t, orbs)
				if !ok {
					continue
				}
				if best == nil || hit.Orb < best.Orb {
					h := hit
					best = &h
				}
			}
			if best != nil {
				found = append(found, *best)
			}
		}
	}

	// Tightest first: that is the order a reader wants, and it makes the
	// response stable regardless of the order bodies were requested in.
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].Orb != found[j].Orb {
			return found[i].Orb < found[j].Orb
		}
		if found[i].From != found[j].From {
			return found[i].From < found[j].From
		}
		return found[i].To < found[j].To
	})
	return found
}

// aspectBetween tests one pair against one aspect type.
func aspectBetween(a, b Position, t AspectType, orbs map[string]float64) (Aspect, bool) {
	separation := Separation(a.Longitude, b.Longitude)
	orb := math.Abs(separation - t.Angle)

	// The orb a pair is allowed is the wider of the two bodies', scaled by
	// the aspect. Taking the wider one is what keeps the luminaries' generous
	// orbs against slow bodies, which is what practitioners expect.
	maxOrb := math.Max(a.orb(orbs), b.orb(orbs)) * t.OrbFactor
	if orb > maxOrb {
		return Aspect{}, false
	}

	return Aspect{
		From:       a.Body,
		To:         b.Body,
		Type:       t.Name,
		Angle:      t.Angle,
		Separation: separation,
		Orb:        orb,
		MaxOrb:     maxOrb,
		IsApplying: isApplying(a, b, t.Angle),
		IsExact:    orb < exactThreshold,
	}, true
}

// isApplying reports whether the two bodies are closing on the exact angle.
//
// It advances both by their own speed and asks whether the gap narrowed. Doing
// it this way rather than by reasoning about signs and directions means
// conjunctions, oppositions, retrograde motion and a faster body overtaking a
// slower one all fall out of the same comparison.
func isApplying(a, b Position, angle float64) bool {
	now := math.Abs(Separation(a.Longitude, b.Longitude) - angle)
	later := math.Abs(Separation(
		a.Longitude+a.Speed*lookaheadDays,
		b.Longitude+b.Speed*lookaheadDays,
	) - angle)
	return later < now
}

// Distribution counts how the bodies fall across the elements, the modalities
// and the two polarities. It is the summary most chart readings open with.
type Distribution struct {
	Elements   map[string]int `json:"elements"`
	Modalities map[string]int `json:"modalities"`
	Polarities map[string]int `json:"polarities"`
}

// Distribute counts the given positions by sign quality.
//
// Only bodies are counted. Including the angles would double count the
// ascendant and midheaven, which are already reflected in the houses, and
// inflate whichever element they happen to fall in.
func Distribute(positions []Position) Distribution {
	d := Distribution{
		Elements:   map[string]int{"fire": 0, "earth": 0, "air": 0, "water": 0},
		Modalities: map[string]int{"cardinal": 0, "fixed": 0, "mutable": 0},
		Polarities: map[string]int{"positive": 0, "negative": 0},
	}

	for _, p := range positions {
		if b, ok := LookupBody(p.Body); ok && b.Category == CategoryAngle {
			continue
		}
		s := signs[p.SignIndex]
		d.Elements[s.Element]++
		d.Modalities[s.Modality]++
		d.Polarities[s.Polarity]++
	}
	return d
}
