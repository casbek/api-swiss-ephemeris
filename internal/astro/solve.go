package astro

import "math"

// Finding when something happens in the sky means solving for a moment rather
// than reading one off. There is no closed form for "when does Mars reach 14
// Leo", so the answer is searched for: step along until the quantity changes
// sign, then close in on the crossing.

const (
	// solverTolerance is how finely a crossing is located, in days. A tenth
	// of a second, far finer than any birth time is ever known to.
	solverTolerance = 1e-6

	// maxBisections bounds the closing-in. Each step halves the bracket, so
	// sixty covers any interval that could be bracketed at all.
	maxBisections = 60

	// maxNewtonSteps bounds the refinement of an estimate that is already
	// close. Convergence is quadratic, so a handful is plenty.
	maxNewtonSteps = 12

	// newtonTolerance is the angular precision a refined crossing must reach,
	// in degrees. A thousandth of an arcsecond.
	newtonTolerance = 1e-9
)

// longitudeFunc reports a body's longitude and its motion per day at a moment.
type longitudeFunc func(jd float64) (longitude, speed float64, err error)

// crossing finds the first moment at or after start when the longitude reaches
// target, searching forward for at most window days.
//
// step has to be small enough that the body cannot pass the target and come
// back within one step. The Moon covers thirteen degrees a day, so a quarter
// of a day is a safe step for it and generous for everything slower.
func crossing(f longitudeFunc, target, start, window, step float64) (float64, bool, error) {
	prev, _, err := f(start)
	if err != nil {
		return 0, false, err
	}
	prevDiff := NormalizeSigned(prev - target)
	if prevDiff == 0 {
		return start, true, nil
	}

	for t := start + step; t <= start+window+step; t += step {
		if t > start+window {
			t = start + window
		}

		lon, _, err := f(t)
		if err != nil {
			return 0, false, err
		}
		diff := NormalizeSigned(lon - target)

		// A sign change brackets a crossing, but only when both values are
		// near zero. The jump from +179 to -179 is the body passing the
		// point opposite the target, which is not a crossing at all.
		if signsDiffer(prevDiff, diff) && math.Abs(prevDiff) < 90 && math.Abs(diff) < 90 {
			root, err := bisect(f, target, t-step, t)
			return root, true, err
		}

		prevDiff = diff
		if t >= start+window {
			break
		}
	}
	return 0, false, nil
}

// bisect closes in on a crossing known to lie between lo and hi.
func bisect(f longitudeFunc, target, lo, hi float64) (float64, error) {
	loLon, _, err := f(lo)
	if err != nil {
		return 0, err
	}
	loDiff := NormalizeSigned(loLon - target)

	for i := 0; i < maxBisections && hi-lo > solverTolerance; i++ {
		mid := (lo + hi) / 2
		midLon, _, err := f(mid)
		if err != nil {
			return 0, err
		}
		midDiff := NormalizeSigned(midLon - target)

		if signsDiffer(loDiff, midDiff) {
			hi = mid
		} else {
			lo, loDiff = mid, midDiff
		}
	}
	return (lo + hi) / 2, nil
}

// refine turns a rough estimate of a crossing into an exact one, by stepping
// along the body's own motion towards the target.
//
// This is for a crossing that is already known to be nearby, where stepping
// along a whole window would be wasted work. It gives up rather than wander:
// a body that is stationary, or one whose crossing turns out to be further off
// than maxDays, gets no answer instead of a wrong one.
func refine(f longitudeFunc, target, guess, maxDays float64) (float64, bool, error) {
	t := guess

	for i := 0; i < maxNewtonSteps; i++ {
		lon, speed, err := f(t)
		if err != nil {
			return 0, false, err
		}

		diff := NormalizeSigned(lon - target)
		if math.Abs(diff) < newtonTolerance {
			return t, true, nil
		}
		// A body at a station has no motion to carry it to the target, and
		// dividing by that speed would throw the estimate anywhere.
		if math.Abs(speed) < 1e-7 {
			return 0, false, nil
		}

		t -= diff / speed
		if math.Abs(t-guess) > maxDays {
			return 0, false, nil
		}
	}

	// Ran out of steps without converging. Report nothing rather than an
	// answer that has not settled.
	return 0, false, nil
}

// signsDiffer reports whether two values lie on opposite sides of zero.
func signsDiffer(a, b float64) bool { return (a < 0) != (b < 0) }
