package astro

import (
	"testing"

	"github.com/casbek/api-swiss-ephemeris/internal/libswe"
)

// TestCatalogueIsConsistent guards the invariants the rest of the service
// relies on. The reference endpoints publish this catalogue, so an entry that
// is advertised but cannot be calculated would become a broken promise.
func TestCatalogueIsConsistent(t *testing.T) {
	seen := map[string]bool{}
	for _, b := range Bodies() {
		if seen[b.Name] {
			t.Errorf("duplicate body name %q", b.Name)
		}
		seen[b.Name] = true

		if b.BaseOrb <= 0 {
			t.Errorf("%s has base orb %g, want a positive value", b.Name, b.BaseOrb)
		}
		if _, ok := LookupBody(b.Name); !ok {
			t.Errorf("%s is listed but cannot be looked up", b.Name)
		}
	}

	for _, a := range Angles() {
		if !a.Derived() {
			t.Errorf("%s is an angle and must be derived, not requested from the ephemeris", a.Name)
		}
		if _, ok := LookupBody(a.Name); !ok {
			t.Errorf("%s is listed but cannot be looked up", a.Name)
		}
	}
}

// TestSwissIDsAreCorrect spot checks the mapping to Swiss Ephemeris. A wrong
// id here would silently return the position of a different body.
func TestSwissIDsAreCorrect(t *testing.T) {
	want := map[string]int{
		"sun":        libswe.Sun,
		"moon":       libswe.Moon,
		"mercury":    libswe.Mercury,
		"pluto":      libswe.Pluto,
		"true_node":  libswe.TrueNode,
		"mean_node":  libswe.MeanNode,
		"chiron":     libswe.Chiron,
		"lilith":     libswe.MeanApog,
		"south_node": -1, // derived from the north node
	}
	for name, id := range want {
		b, ok := LookupBody(name)
		if !ok {
			t.Errorf("%s is missing from the catalogue", name)
			continue
		}
		if b.SwissID != id {
			t.Errorf("%s has Swiss id %d, want %d", name, b.SwissID, id)
		}
	}
}

func TestDefaultSets(t *testing.T) {
	bodies := DefaultBodies()
	if len(bodies) != 13 {
		t.Errorf("the default body set has %d entries, want 13", len(bodies))
	}
	for _, name := range bodies {
		if _, ok := LookupBody(name); !ok {
			t.Errorf("the default set names %q, which is not in the catalogue", name)
		}
	}

	aspects := DefaultAspects()
	if len(aspects) != 5 {
		t.Errorf("the default aspect set has %d entries, want the 5 major aspects", len(aspects))
	}
	for _, name := range aspects {
		a, ok := LookupAspect(name)
		if !ok {
			t.Errorf("the default set names aspect %q, which is not in the catalogue", name)
			continue
		}
		if a.Category != "major" {
			t.Errorf("%s is in the default set but categorised as %s", name, a.Category)
		}
	}
}

func TestDefaultsResolve(t *testing.T) {
	if _, ok := LookupHouseSystem(DefaultHouseSystem); !ok {
		t.Errorf("the default house system %q is not in the catalogue", DefaultHouseSystem)
	}
	if _, ok := LookupAyanamsha(DefaultAyanamsha); !ok {
		t.Errorf("the default ayanamsha %q is not in the catalogue", DefaultAyanamsha)
	}
}

// TestHouseSystemLettersAreUnique checks the codes passed to Swiss Ephemeris.
// A duplicate would mean two names quietly producing the same chart.
func TestHouseSystemLettersAreUnique(t *testing.T) {
	seen := map[byte]string{}
	for _, h := range HouseSystems() {
		if h.Letter == 0 {
			t.Errorf("%s has no letter", h.Name)
		}
		if prev, dup := seen[h.Letter]; dup {
			t.Errorf("%s and %s share the letter %q", prev, h.Name, h.Letter)
		}
		seen[h.Letter] = h.Name
	}

	// The systems documented as undefined near the poles must be flagged, so
	// a request from a high latitude can be refused rather than answered
	// with degenerate cusps.
	for _, name := range []string{"placidus", "koch"} {
		h, ok := LookupHouseSystem(name)
		if !ok {
			t.Fatalf("%s is missing", name)
		}
		if !h.FailsAtHighLatitude {
			t.Errorf("%s is not flagged as failing at high latitude", name)
		}
	}
	// Whole sign works everywhere and must not be flagged.
	if h, _ := LookupHouseSystem("whole_sign"); h.FailsAtHighLatitude {
		t.Error("whole_sign is flagged as failing at high latitude, but it is defined everywhere")
	}
}

func TestSignsCoverTheZodiac(t *testing.T) {
	signs := Signs()
	if len(signs) != 12 {
		t.Fatalf("there are %d signs, want 12", len(signs))
	}

	elements := map[string]int{}
	modalities := map[string]int{}
	for i, s := range signs {
		if s.Index != i {
			t.Errorf("%s has index %d, want %d", s.Name, s.Index, i)
		}
		if want := float64(i) * 30; s.StartLongitude != want {
			t.Errorf("%s starts at %g, want %g", s.Name, s.StartLongitude, want)
		}
		if _, ok := LookupBody(s.Ruler); !ok {
			t.Errorf("%s is ruled by %q, which is not a known body", s.Name, s.Ruler)
		}
		if s.ModernRuler != "" {
			if _, ok := LookupBody(s.ModernRuler); !ok {
				t.Errorf("%s has modern ruler %q, which is not a known body", s.Name, s.ModernRuler)
			}
		}
		elements[s.Element]++
		modalities[s.Modality]++
	}

	// Each element covers three signs and each modality four.
	for _, e := range []string{"fire", "earth", "air", "water"} {
		if elements[e] != 3 {
			t.Errorf("element %s covers %d signs, want 3", e, elements[e])
		}
	}
	for _, m := range []string{"cardinal", "fixed", "mutable"} {
		if modalities[m] != 4 {
			t.Errorf("modality %s covers %d signs, want 4", m, modalities[m])
		}
	}
}

func TestLookupIsCaseInsensitive(t *testing.T) {
	if _, ok := LookupBody("  SUN "); !ok {
		t.Error("body lookup should trim and lowercase its input")
	}
	if _, ok := LookupHouseSystem("Placidus"); !ok {
		t.Error("house system lookup should be case insensitive")
	}
	if _, ok := LookupAspect("TRINE"); !ok {
		t.Error("aspect lookup should be case insensitive")
	}
}
