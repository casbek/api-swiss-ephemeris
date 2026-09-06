// Package astro holds the astrological domain model: the catalogue of bodies,
// signs, house systems, aspects and ayanamshas, and the chart calculations
// built on them.
//
// The catalogue is the single source for these tables. The reference endpoints
// serve it directly, and the chart code resolves names through it, so a body
// or a house system can never be advertised without being supported.
package astro

import (
	"sort"
	"strings"

	"github.com/casbek/api-swiss-ephemeris/internal/libswe"
)

// Category groups bodies for presentation.
type Category string

const (
	CategoryLuminary Category = "luminary"
	CategoryPlanet   Category = "planet"
	CategoryPoint    Category = "point"
	CategoryAsteroid Category = "asteroid"
	CategoryAngle    Category = "angle"
)

// Body is a celestial body or calculated point that can be requested.
type Body struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Category Category `json:"category"`

	// BaseOrb is the orb this body is allowed in degrees, before the aspect
	// factor is applied. See Aspect.OrbFactor.
	BaseOrb float64 `json:"base_orb"`

	// SwissID is the ipl value passed to Swiss Ephemeris, or -1 for points
	// that are derived from another result rather than computed directly.
	SwissID int `json:"-"`

	// InDefaultSet reports whether the body is returned when a request does
	// not list bodies explicitly.
	InDefaultSet bool `json:"in_default_set"`
}

// Derived reports whether the body is calculated from another value instead of
// being asked of Swiss Ephemeris directly.
func (b Body) Derived() bool { return b.SwissID < 0 }

// bodies is the catalogue in presentation order.
var bodies = []Body{
	{Name: "sun", Label: "Sun", Category: CategoryLuminary, BaseOrb: 10, SwissID: libswe.Sun, InDefaultSet: true},
	{Name: "moon", Label: "Moon", Category: CategoryLuminary, BaseOrb: 10, SwissID: libswe.Moon, InDefaultSet: true},

	{Name: "mercury", Label: "Mercury", Category: CategoryPlanet, BaseOrb: 7, SwissID: libswe.Mercury, InDefaultSet: true},
	{Name: "venus", Label: "Venus", Category: CategoryPlanet, BaseOrb: 7, SwissID: libswe.Venus, InDefaultSet: true},
	{Name: "mars", Label: "Mars", Category: CategoryPlanet, BaseOrb: 7, SwissID: libswe.Mars, InDefaultSet: true},
	{Name: "jupiter", Label: "Jupiter", Category: CategoryPlanet, BaseOrb: 7, SwissID: libswe.Jupiter, InDefaultSet: true},
	{Name: "saturn", Label: "Saturn", Category: CategoryPlanet, BaseOrb: 7, SwissID: libswe.Saturn, InDefaultSet: true},
	{Name: "uranus", Label: "Uranus", Category: CategoryPlanet, BaseOrb: 6, SwissID: libswe.Uranus, InDefaultSet: true},
	{Name: "neptune", Label: "Neptune", Category: CategoryPlanet, BaseOrb: 6, SwissID: libswe.Neptune, InDefaultSet: true},
	{Name: "pluto", Label: "Pluto", Category: CategoryPlanet, BaseOrb: 6, SwissID: libswe.Pluto, InDefaultSet: true},

	{Name: "true_node", Label: "True Node", Category: CategoryPoint, BaseOrb: 4, SwissID: libswe.TrueNode, InDefaultSet: true},
	{Name: "mean_node", Label: "Mean Node", Category: CategoryPoint, BaseOrb: 4, SwissID: libswe.MeanNode},
	// The south node is always opposite the north node, so it is derived
	// rather than computed.
	{Name: "south_node", Label: "South Node", Category: CategoryPoint, BaseOrb: 4, SwissID: -1},
	{Name: "chiron", Label: "Chiron", Category: CategoryPoint, BaseOrb: 4, SwissID: libswe.Chiron, InDefaultSet: true},
	{Name: "lilith", Label: "Lilith", Category: CategoryPoint, BaseOrb: 4, SwissID: libswe.MeanApog, InDefaultSet: true},
	{Name: "true_lilith", Label: "True Lilith", Category: CategoryPoint, BaseOrb: 4, SwissID: libswe.OscuApog},
	{Name: "earth", Label: "Earth", Category: CategoryPoint, BaseOrb: 4, SwissID: libswe.Earth},

	{Name: "ceres", Label: "Ceres", Category: CategoryAsteroid, BaseOrb: 4, SwissID: libswe.Ceres},
	{Name: "pallas", Label: "Pallas", Category: CategoryAsteroid, BaseOrb: 4, SwissID: libswe.Pallas},
	{Name: "juno", Label: "Juno", Category: CategoryAsteroid, BaseOrb: 4, SwissID: libswe.Juno},
	{Name: "vesta", Label: "Vesta", Category: CategoryAsteroid, BaseOrb: 4, SwissID: libswe.Vesta},
	{Name: "pholus", Label: "Pholus", Category: CategoryAsteroid, BaseOrb: 4, SwissID: libswe.Pholus},
}

// angles are the chart points that come from the house calculation rather than
// from an ephemeris lookup.
var angles = []Body{
	{Name: "ascendant", Label: "Ascendant", Category: CategoryAngle, BaseOrb: 8, SwissID: -1},
	{Name: "midheaven", Label: "Midheaven", Category: CategoryAngle, BaseOrb: 8, SwissID: -1},
	{Name: "descendant", Label: "Descendant", Category: CategoryAngle, BaseOrb: 8, SwissID: -1},
	{Name: "imum_coeli", Label: "Imum Coeli", Category: CategoryAngle, BaseOrb: 8, SwissID: -1},
	{Name: "vertex", Label: "Vertex", Category: CategoryAngle, BaseOrb: 4, SwissID: -1},
}

var bodyIndex = func() map[string]Body {
	m := make(map[string]Body, len(bodies)+len(angles))
	for _, b := range bodies {
		m[b.Name] = b
	}
	for _, a := range angles {
		m[a.Name] = a
	}
	return m
}()

// Bodies returns the catalogue of requestable bodies.
func Bodies() []Body { return append([]Body(nil), bodies...) }

// Angles returns the chart angles.
func Angles() []Body { return append([]Body(nil), angles...) }

// LookupBody finds a body or angle by name.
func LookupBody(name string) (Body, bool) {
	b, ok := bodyIndex[strings.ToLower(strings.TrimSpace(name))]
	return b, ok
}

// DefaultBodies returns the names returned when a request does not list any.
func DefaultBodies() []string {
	var out []string
	for _, b := range bodies {
		if b.InDefaultSet {
			out = append(out, b.Name)
		}
	}
	return out
}

// Sign is one of the twelve zodiac signs.
type Sign struct {
	Name string `json:"name"`
	// Index is zero based from Aries.
	Index          int     `json:"index"`
	Element        string  `json:"element"`
	Modality       string  `json:"modality"`
	Polarity       string  `json:"polarity"`
	Ruler          string  `json:"ruler"`
	ModernRuler    string  `json:"modern_ruler,omitempty"`
	Symbol         string  `json:"symbol"`
	StartLongitude float64 `json:"start_longitude"`
}

// signs lists the zodiac in order. Ruler is the traditional ruler; ModernRuler
// is set only where modern practice assigns a different one.
var signs = [12]Sign{
	{Name: "aries", Index: 0, Element: "fire", Modality: "cardinal", Polarity: "positive", Ruler: "mars", Symbol: "♈"},
	{Name: "taurus", Index: 1, Element: "earth", Modality: "fixed", Polarity: "negative", Ruler: "venus", Symbol: "♉"},
	{Name: "gemini", Index: 2, Element: "air", Modality: "mutable", Polarity: "positive", Ruler: "mercury", Symbol: "♊"},
	{Name: "cancer", Index: 3, Element: "water", Modality: "cardinal", Polarity: "negative", Ruler: "moon", Symbol: "♋"},
	{Name: "leo", Index: 4, Element: "fire", Modality: "fixed", Polarity: "positive", Ruler: "sun", Symbol: "♌"},
	{Name: "virgo", Index: 5, Element: "earth", Modality: "mutable", Polarity: "negative", Ruler: "mercury", Symbol: "♍"},
	{Name: "libra", Index: 6, Element: "air", Modality: "cardinal", Polarity: "positive", Ruler: "venus", Symbol: "♎"},
	{Name: "scorpio", Index: 7, Element: "water", Modality: "fixed", Polarity: "negative", Ruler: "mars", ModernRuler: "pluto", Symbol: "♏"},
	{Name: "sagittarius", Index: 8, Element: "fire", Modality: "mutable", Polarity: "positive", Ruler: "jupiter", Symbol: "♐"},
	{Name: "capricorn", Index: 9, Element: "earth", Modality: "cardinal", Polarity: "negative", Ruler: "saturn", Symbol: "♑"},
	{Name: "aquarius", Index: 10, Element: "air", Modality: "fixed", Polarity: "positive", Ruler: "saturn", ModernRuler: "uranus", Symbol: "♒"},
	{Name: "pisces", Index: 11, Element: "water", Modality: "mutable", Polarity: "negative", Ruler: "jupiter", ModernRuler: "neptune", Symbol: "♓"},
}

// Signs returns the zodiac in order, with each sign's starting longitude set.
func Signs() []Sign {
	out := make([]Sign, len(signs))
	for i, s := range signs {
		s.StartLongitude = float64(i) * 30
		out[i] = s
	}
	return out
}

// SignAt returns the sign containing a longitude in degrees.
func SignAt(longitude float64) Sign {
	i := int(Normalize(longitude) / 30)
	if i > 11 {
		i = 11 // guard against a longitude of exactly 360 after rounding
	}
	s := signs[i]
	s.StartLongitude = float64(i) * 30
	return s
}

// HouseSystem is a method of dividing the chart into houses.
type HouseSystem struct {
	Name  string `json:"name"`
	Label string `json:"label"`

	// Letter is the code Swiss Ephemeris expects. It is not part of the API.
	Letter byte `json:"-"`

	// FailsAtHighLatitude marks systems that are undefined near the poles,
	// where the ecliptic can fail to intersect the horizon as they require.
	FailsAtHighLatitude bool `json:"fails_at_high_latitude"`
}

// houseSystems covers every system Swiss Ephemeris implements. The letters are
// taken from the switch in swehouse.c rather than from documentation.
var houseSystems = []HouseSystem{
	{Name: "placidus", Label: "Placidus", Letter: 'P', FailsAtHighLatitude: true},
	{Name: "koch", Label: "Koch", Letter: 'K', FailsAtHighLatitude: true},
	{Name: "whole_sign", Label: "Whole sign", Letter: 'W'},
	{Name: "equal", Label: "Equal", Letter: 'A'},
	{Name: "equal_mc", Label: "Equal from Midheaven", Letter: 'D'},
	{Name: "equal_aries", Label: "Equal from 0° Aries", Letter: 'N'},
	{Name: "vehlow", Label: "Vehlow equal", Letter: 'V'},
	{Name: "porphyry", Label: "Porphyry", Letter: 'O'},
	{Name: "regiomontanus", Label: "Regiomontanus", Letter: 'R'},
	{Name: "campanus", Label: "Campanus", Letter: 'C'},
	{Name: "alcabitius", Label: "Alcabitius", Letter: 'B'},
	{Name: "morinus", Label: "Morinus", Letter: 'M'},
	{Name: "polich_page", Label: "Polich-Page (topocentric)", Letter: 'T', FailsAtHighLatitude: true},
	{Name: "krusinski", Label: "Krusinski-Pisa-Goelzer", Letter: 'U'},
	{Name: "sripati", Label: "Sripati", Letter: 'S'},
	{Name: "sunshine", Label: "Sunshine", Letter: 'I'},
	{Name: "pullen_sd", Label: "Pullen sinusoidal delta", Letter: 'L'},
	{Name: "pullen_sr", Label: "Pullen sinusoidal ratio", Letter: 'Q'},
	{Name: "carter", Label: "Carter poli-equatorial", Letter: 'F'},
	{Name: "savard", Label: "Savard A", Letter: 'J'},
	{Name: "horizon", Label: "Horizon / azimuthal", Letter: 'H'},
	{Name: "meridian", Label: "Meridian (axial rotation)", Letter: 'X'},
	{Name: "apc", Label: "APC houses", Letter: 'Y'},
	{Name: "gauquelin", Label: "Gauquelin sectors", Letter: 'G'},
}

var houseSystemIndex = func() map[string]HouseSystem {
	m := make(map[string]HouseSystem, len(houseSystems))
	for _, h := range houseSystems {
		m[h.Name] = h
	}
	return m
}()

// DefaultHouseSystem is used when a request does not name one.
const DefaultHouseSystem = "placidus"

// HouseSystems returns the supported house systems.
func HouseSystems() []HouseSystem { return append([]HouseSystem(nil), houseSystems...) }

// LookupHouseSystem finds a house system by name.
func LookupHouseSystem(name string) (HouseSystem, bool) {
	h, ok := houseSystemIndex[strings.ToLower(strings.TrimSpace(name))]
	return h, ok
}

// Aspect is an angular relationship between two positions.
type Aspect struct {
	Name  string  `json:"name"`
	Angle float64 `json:"angle"`

	// OrbFactor scales the orb allowed for this aspect. The orb of a pair is
	// the larger base orb of the two bodies multiplied by this factor.
	OrbFactor float64 `json:"orb_factor"`

	Category     string `json:"category"`
	InDefaultSet bool   `json:"in_default_set"`
}

var aspects = []Aspect{
	{Name: "conjunction", Angle: 0, OrbFactor: 1.0, Category: "major", InDefaultSet: true},
	{Name: "opposition", Angle: 180, OrbFactor: 1.0, Category: "major", InDefaultSet: true},
	{Name: "trine", Angle: 120, OrbFactor: 1.0, Category: "major", InDefaultSet: true},
	{Name: "square", Angle: 90, OrbFactor: 1.0, Category: "major", InDefaultSet: true},
	{Name: "sextile", Angle: 60, OrbFactor: 0.75, Category: "major", InDefaultSet: true},

	{Name: "quincunx", Angle: 150, OrbFactor: 0.4, Category: "minor"},
	{Name: "semisextile", Angle: 30, OrbFactor: 0.4, Category: "minor"},
	{Name: "semisquare", Angle: 45, OrbFactor: 0.4, Category: "minor"},
	{Name: "sesquiquadrate", Angle: 135, OrbFactor: 0.4, Category: "minor"},
	{Name: "quintile", Angle: 72, OrbFactor: 0.3, Category: "minor"},
	{Name: "biquintile", Angle: 144, OrbFactor: 0.3, Category: "minor"},
}

var aspectIndex = func() map[string]Aspect {
	m := make(map[string]Aspect, len(aspects))
	for _, a := range aspects {
		m[a.Name] = a
	}
	return m
}()

// Aspects returns the supported aspects.
func Aspects() []Aspect { return append([]Aspect(nil), aspects...) }

// LookupAspect finds an aspect by name.
func LookupAspect(name string) (Aspect, bool) {
	a, ok := aspectIndex[strings.ToLower(strings.TrimSpace(name))]
	return a, ok
}

// DefaultAspects returns the aspect names used when a request lists none.
func DefaultAspects() []string {
	var out []string
	for _, a := range aspects {
		if a.InDefaultSet {
			out = append(out, a.Name)
		}
	}
	return out
}

// Ayanamsha is an offset between the tropical and sidereal zodiacs.
type Ayanamsha struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	SwissID int32  `json:"-"`
}

var ayanamshas = []Ayanamsha{
	{Name: "lahiri", Label: "Lahiri", SwissID: libswe.SidmLahiri},
	{Name: "lahiri_icrc", Label: "Lahiri ICRC", SwissID: libswe.SidmLahiriICRC},
	{Name: "fagan_bradley", Label: "Fagan-Bradley", SwissID: libswe.SidmFaganBradley},
	{Name: "raman", Label: "Raman", SwissID: libswe.SidmRaman},
	{Name: "krishnamurti", Label: "Krishnamurti", SwissID: libswe.SidmKrishnamurti},
	{Name: "yukteshwar", Label: "Yukteshwar", SwissID: libswe.SidmYukteshwar},
	{Name: "jn_bhasin", Label: "J. N. Bhasin", SwissID: libswe.SidmJNBhasin},
	{Name: "deluce", Label: "De Luce", SwissID: libswe.SidmDeluce},
	{Name: "true_citra", Label: "True Citra", SwissID: libswe.SidmTrueCitra},
	{Name: "true_revati", Label: "True Revati", SwissID: libswe.SidmTrueRevati},
	{Name: "true_pushya", Label: "True Pushya", SwissID: libswe.SidmTruePushya},
}

var ayanamshaIndex = func() map[string]Ayanamsha {
	m := make(map[string]Ayanamsha, len(ayanamshas))
	for _, a := range ayanamshas {
		m[a.Name] = a
	}
	return m
}()

// DefaultAyanamsha is used when sidereal mode is requested without naming one.
const DefaultAyanamsha = "lahiri"

// Ayanamshas returns the supported ayanamshas.
func Ayanamshas() []Ayanamsha { return append([]Ayanamsha(nil), ayanamshas...) }

// LookupAyanamsha finds an ayanamsha by name.
func LookupAyanamsha(name string) (Ayanamsha, bool) {
	a, ok := ayanamshaIndex[strings.ToLower(strings.TrimSpace(name))]
	return a, ok
}

// KnownBodyNames returns every accepted body and angle name, sorted. It exists
// to build helpful error messages.
func KnownBodyNames() []string {
	out := make([]string, 0, len(bodyIndex))
	for name := range bodyIndex {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
