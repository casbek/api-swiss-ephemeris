package astro

import (
	"math"
	"sort"
)

// CrossAspects finds the aspects between two sets of positions.
//
// Unlike FindAspects it never compares a set with itself: a transiting Sun is
// compared with every natal point, but not with the other transiting bodies,
// whose aspects belong to the transit chart rather than to the comparison.
//
// From always names a point in the first set and To a point in the second, so
// the direction is fixed and a caller can label the two sides.
func CrossAspects(from, to []Position, types []AspectType, orbs map[string]float64) []Aspect {
	found := make([]Aspect, 0)

	for _, a := range from {
		for _, b := range to {
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

// Midpoint returns the point halfway between two longitudes, taking the
// shorter way round.
//
// Two points on a circle have two midpoints, half a turn apart. Composite
// charts and midpoint work both mean the near one, which is the convention
// this follows. For a pair that is exactly opposite the two are equally near
// and the result is arbitrary but consistent.
func Midpoint(a, b float64) float64 {
	return Normalize(a + NormalizeSigned(b-a)/2)
}

// midpointPositions pairs up two sets of positions by body and returns the
// midpoint of each, keeping only bodies present in both.
func midpointPositions(a, b []Position) []Position {
	index := make(map[string]Position, len(b))
	for _, p := range b {
		index[p.Body] = p
	}

	out := make([]Position, 0, len(a))
	for _, pa := range a {
		pb, ok := index[pa.Body]
		if !ok {
			continue
		}

		p := newPosition(pa.Body, pa.Label, Midpoint(pa.Longitude, pb.Longitude))
		p.Latitude = (pa.Latitude + pb.Latitude) / 2
		p.DistanceAU = (pa.DistanceAU + pb.DistanceAU) / 2
		// A composite has no motion of its own. Averaging the two speeds is
		// the usual convention and keeps the retrograde flag meaningful.
		p.Speed = (pa.Speed + pb.Speed) / 2
		p.IsRetrograde = p.Speed < 0
		out = append(out, p)
	}
	return out
}

// meanLatitude returns the midpoint of two geographic latitudes. Latitude is
// bounded rather than circular, so this is a plain average.
func meanLatitude(a, b float64) float64 { return (a + b) / 2 }

// meanLongitude returns the midpoint of two geographic longitudes, taking the
// shorter way round the globe.
//
// A plain average of Tokyo at 139.69 east and Los Angeles at 118.24 west gives
// 10.7 east, in the Mediterranean, when the point that actually lies between
// them is in the Pacific, half a world away.
func meanLongitude(a, b float64) float64 {
	mid := Midpoint(Normalize(a), Normalize(b))
	if mid > 180 {
		mid -= 360
	}
	return mid
}

// cuspsOutOfOrder reports whether a set of cusps fails to run forward around
// the circle, which would mean the division is degenerate.
func cuspsOutOfOrder(cusps []Cusp) bool {
	if len(cusps) < 2 {
		return false
	}
	total := 0.0
	for i := range cusps {
		next := cusps[(i+1)%len(cusps)]
		span := Normalize(next.Longitude - cusps[i].Longitude)
		if span <= 0 || span >= 180 {
			return true
		}
		total += span
	}
	return math.Abs(total-360) > 1e-6
}
