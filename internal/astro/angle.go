package astro

import (
	"fmt"
	"math"
)

// Normalize folds an angle into [0, 360).
func Normalize(deg float64) float64 {
	d := math.Mod(deg, 360)
	if d < 0 {
		d += 360
	}
	// math.Mod on a tiny negative value can round to exactly 360 after the
	// addition above, which would fall outside the range.
	if d >= 360 {
		d = 0
	}
	return d
}

// NormalizeSigned folds an angle into [-180, 180), the form used for a
// difference between two positions.
func NormalizeSigned(deg float64) float64 {
	d := Normalize(deg)
	if d >= 180 {
		d -= 360
	}
	return d
}

// Separation returns the shortest angular distance between two longitudes, in
// [0, 180].
func Separation(a, b float64) float64 {
	return math.Abs(NormalizeSigned(a - b))
}

// DegreeInSign returns the position within its sign, in [0, 30).
func DegreeInSign(longitude float64) float64 {
	return math.Mod(Normalize(longitude), 30)
}

// DMS splits a positive angle into degrees, minutes and seconds. Seconds are
// rounded to the nearest whole second, carrying into minutes and degrees so
// the result never reads 59'60".
func DMS(deg float64) (d, m, s int) {
	total := int(math.Round(math.Abs(deg) * 3600))
	d = total / 3600
	m = (total % 3600) / 60
	s = total % 60
	return d, m, s
}

// FormatDMS renders an angle as degrees, minutes and seconds.
func FormatDMS(deg float64) string {
	d, m, s := DMS(deg)
	return fmt.Sprintf("%d°%02d'%02d\"", d, m, s)
}

// FormatPosition renders a longitude the way astrologers write it, as the
// degree within the sign followed by the sign name.
func FormatPosition(longitude float64) string {
	sign := SignAt(longitude)
	d, m, s := DMS(DegreeInSign(longitude))
	return fmt.Sprintf("%d°%02d'%02d\" %s", d, m, s, sign.Label())
}

// Label returns the sign name with an initial capital.
func (s Sign) Label() string {
	if s.Name == "" {
		return ""
	}
	return string(s.Name[0]-32) + s.Name[1:]
}
