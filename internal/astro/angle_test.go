package astro

import (
	"math"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := []struct {
		in, want float64
	}{
		{0, 0},
		{84.229, 84.229},
		{360, 0},
		{361, 1},
		{720, 0},
		{-1, 359},
		{-360, 0},
		{-361, 359},
		// A tiny negative value must not round up to exactly 360, which
		// would fall outside the half open range.
		{-1e-15, 0},
	}
	for _, tc := range cases {
		got := Normalize(tc.in)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("Normalize(%g) = %g, want %g", tc.in, got, tc.want)
		}
		if got < 0 || got >= 360 {
			t.Errorf("Normalize(%g) = %g, outside [0, 360)", tc.in, got)
		}
	}
}

func TestNormalizeSigned(t *testing.T) {
	cases := []struct {
		in, want float64
	}{
		{0, 0},
		{90, 90},
		{179, 179},
		{180, -180},
		{181, -179},
		{270, -90},
		{359, -1},
		{-90, -90},
	}
	for _, tc := range cases {
		got := NormalizeSigned(tc.in)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("NormalizeSigned(%g) = %g, want %g", tc.in, got, tc.want)
		}
	}
}

func TestSeparation(t *testing.T) {
	cases := []struct {
		a, b, want float64
	}{
		{0, 0, 0},
		{0, 90, 90},
		{0, 180, 180},
		// The short way round: 10 degrees apart across 0 Aries, not 350.
		{355, 5, 10},
		{5, 355, 10},
		{120, 240, 120},
	}
	for _, tc := range cases {
		got := Separation(tc.a, tc.b)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("Separation(%g, %g) = %g, want %g", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestSignAt(t *testing.T) {
	cases := []struct {
		longitude float64
		want      string
	}{
		{0, "aries"},
		{29.999, "aries"},
		{30, "taurus"},
		{84.2290447, "gemini"},
		{180, "libra"},
		{359.999, "pisces"},
		{360, "aries"},
		{-1, "pisces"},
	}
	for _, tc := range cases {
		if got := SignAt(tc.longitude).Name; got != tc.want {
			t.Errorf("SignAt(%g) = %q, want %q", tc.longitude, got, tc.want)
		}
	}
}

func TestDegreeInSign(t *testing.T) {
	cases := []struct {
		longitude, want float64
	}{
		{0, 0},
		{29.5, 29.5},
		{30, 0},
		{84.2290447, 24.2290447},
		{359.5, 29.5},
	}
	for _, tc := range cases {
		got := DegreeInSign(tc.longitude)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("DegreeInSign(%g) = %g, want %g", tc.longitude, got, tc.want)
		}
	}
}

// TestDMSCarries checks that rounding seconds never produces 60, which would
// render as 59'60" instead of carrying into the next minute.
func TestDMSCarries(t *testing.T) {
	cases := []struct {
		in      float64
		d, m, s int
	}{
		{0, 0, 0, 0},
		{24.2290447, 24, 13, 45},
		// 0.9999999 degrees is 59'59.99964", which must round up to a whole
		// degree rather than to 0°59'60".
		{0.9999999, 1, 0, 0},
		// 29.99999 degrees is one hundredth of a second short of 30.
		{29.999999, 30, 0, 0},
		{1.5, 1, 30, 0},
	}
	for _, tc := range cases {
		d, m, s := DMS(tc.in)
		if d != tc.d || m != tc.m || s != tc.s {
			t.Errorf("DMS(%g) = %d°%02d'%02d\", want %d°%02d'%02d\"",
				tc.in, d, m, s, tc.d, tc.m, tc.s)
		}
		if m > 59 || s > 59 {
			t.Errorf("DMS(%g) produced %d'%d\", which is not a valid reading", tc.in, m, s)
		}
	}
}

func TestFormatPosition(t *testing.T) {
	// The reference Sun position for 1990-06-15 14:30 UT.
	if got, want := FormatPosition(84.2290447), `24°13'45" Gemini`; got != want {
		t.Errorf("FormatPosition = %q, want %q", got, want)
	}
	if got, want := FormatPosition(0), `0°00'00" Aries`; got != want {
		t.Errorf("FormatPosition = %q, want %q", got, want)
	}
}
