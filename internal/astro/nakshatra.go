package astro

import (
	"fmt"
	"math"
)

// The nakshatras are the twenty seven lunar mansions Indian astrology divides
// the zodiac into, each a twenty seventh of the circle, or 13°20'. Each is
// quartered into padas of 3°20'.
//
// They are measured from the sidereal zero point, since they are tied to the
// fixed stars rather than to the equinox. Reading them off a tropical longitude
// would put every one of them about twenty four degrees out, so they are
// reported only for a sidereal chart.

const (
	// NakshatraCount is how many divisions the circle is cut into.
	NakshatraCount = 27

	// NakshatraSpan is the width of one, in degrees: 13°20'.
	NakshatraSpan = 360.0 / NakshatraCount

	// PadasPerNakshatra is how many quarters each is divided into.
	PadasPerNakshatra = 4

	// PadaSpan is the width of one quarter, in degrees: 3°20'.
	PadaSpan = NakshatraSpan / PadasPerNakshatra
)

// vimshottariOrder is the sequence of lords the nakshatras run through, and
// the same sequence the Vimshottari dasha follows. Twenty seven nakshatras
// over nine lords means it repeats exactly three times around the circle.
var vimshottariOrder = [9]string{
	"ketu", "venus", "sun", "moon", "mars", "rahu", "jupiter", "saturn", "mercury",
}

// nakshatraNames lists the mansions in order from the sidereal zero point.
var nakshatraNames = [NakshatraCount]struct {
	Name  string
	Label string
}{
	{"ashwini", "Ashwini"},
	{"bharani", "Bharani"},
	{"krittika", "Krittika"},
	{"rohini", "Rohini"},
	{"mrigashira", "Mrigashira"},
	{"ardra", "Ardra"},
	{"punarvasu", "Punarvasu"},
	{"pushya", "Pushya"},
	{"ashlesha", "Ashlesha"},
	{"magha", "Magha"},
	{"purva_phalguni", "Purva Phalguni"},
	{"uttara_phalguni", "Uttara Phalguni"},
	{"hasta", "Hasta"},
	{"chitra", "Chitra"},
	{"swati", "Swati"},
	{"vishakha", "Vishakha"},
	{"anuradha", "Anuradha"},
	{"jyeshtha", "Jyeshtha"},
	{"mula", "Mula"},
	{"purva_ashadha", "Purva Ashadha"},
	{"uttara_ashadha", "Uttara Ashadha"},
	{"shravana", "Shravana"},
	{"dhanishta", "Dhanishta"},
	{"shatabhisha", "Shatabhisha"},
	{"purva_bhadrapada", "Purva Bhadrapada"},
	{"uttara_bhadrapada", "Uttara Bhadrapada"},
	{"revati", "Revati"},
}

// Nakshatra is one of the twenty seven mansions.
type Nakshatra struct {
	// Number runs from 1 to 27, as the mansions are always cited.
	Number int    `json:"number"`
	Name   string `json:"name"`
	Label  string `json:"label"`

	// Lord is the planet that rules it, and the one whose Vimshottari
	// period a birth under it begins in.
	Lord string `json:"lord"`

	StartLongitude float64 `json:"start_longitude"`
}

// Nakshatras returns all twenty seven in order.
func Nakshatras() []Nakshatra {
	out := make([]Nakshatra, NakshatraCount)
	for i := range out {
		out[i] = nakshatraAt(i)
	}
	return out
}

// nakshatraAt builds the mansion with the given zero-based index.
func nakshatraAt(index int) Nakshatra {
	return Nakshatra{
		Number:         index + 1,
		Name:           nakshatraNames[index].Name,
		Label:          nakshatraNames[index].Label,
		Lord:           vimshottariOrder[index%len(vimshottariOrder)],
		StartLongitude: float64(index) * NakshatraSpan,
	}
}

// NakshatraPlacement is where a position falls among the mansions.
type NakshatraPlacement struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
	Label  string `json:"label"`
	Lord   string `json:"lord"`

	// Pada is the quarter of the mansion, from 1 to 4.
	Pada int `json:"pada"`

	// DegreeIn is how far into the mansion the position lies, in degrees.
	DegreeIn float64 `json:"degree_in"`

	// Fraction is the same as a proportion, from 0 up to but not including
	// 1. The balance of the first dasha is read from it.
	Fraction float64 `json:"fraction"`
}

// NakshatraAt reports which mansion a sidereal longitude falls in.
func NakshatraAt(siderealLongitude float64) NakshatraPlacement {
	lon := Normalize(siderealLongitude)

	index := int(lon / NakshatraSpan)
	if index >= NakshatraCount {
		// Only a longitude that rounded to exactly 360 could reach here.
		index = NakshatraCount - 1
	}
	n := nakshatraAt(index)

	degreeIn := lon - n.StartLongitude
	pada := int(degreeIn/PadaSpan) + 1
	if pada > PadasPerNakshatra {
		pada = PadasPerNakshatra
	}

	return NakshatraPlacement{
		Number:   n.Number,
		Name:     n.Name,
		Label:    n.Label,
		Lord:     n.Lord,
		Pada:     pada,
		DegreeIn: degreeIn,
		Fraction: degreeIn / NakshatraSpan,
	}
}

// FormatNakshatra renders a placement the way it is usually written, as the
// mansion and the quarter.
func FormatNakshatra(p NakshatraPlacement) string {
	d, m, s := DMS(p.DegreeIn)
	return fmt.Sprintf("%s pada %d (%d°%02d'%02d\")", p.Label, p.Pada, d, m, s)
}

// vimshottariYears is how long each lord holds the great period, in years.
// They sum to the hundred and twenty years the whole cycle runs.
var vimshottariYears = map[string]float64{
	"ketu":    7,
	"venus":   20,
	"sun":     6,
	"moon":    10,
	"mars":    7,
	"rahu":    18,
	"jupiter": 16,
	"saturn":  19,
	"mercury": 17,
}

// VimshottariTotalYears is the length of the whole cycle.
const VimshottariTotalYears = 120.0

// vimshottariIndex is where a lord sits in the sequence.
func vimshottariIndex(lord string) int {
	for i, l := range vimshottariOrder {
		if l == lord {
			return i
		}
	}
	return -1
}

// roundTo rounds to a number of decimal places, for reporting a length in
// years without a tail of floating point noise.
func roundTo(v float64, places int) float64 {
	scale := math.Pow(10, float64(places))
	return math.Round(v*scale) / scale
}
