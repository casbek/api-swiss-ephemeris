package astro

import (
	"fmt"
	"sort"
	"strings"

	"github.com/casbek/api-swiss-ephemeris/internal/swe"
	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// Zodiac selects which zodiac a chart is cast in.
type Zodiac string

const (
	// ZodiacTropical measures from the vernal equinox. Western practice.
	ZodiacTropical Zodiac = "tropical"
	// ZodiacSidereal measures from a fixed point among the stars. Vedic
	// practice. The two differ by about 24 degrees today.
	ZodiacSidereal Zodiac = "sidereal"
)

// SettingsInput is the settings object as a client sends it. Every field is
// optional.
type SettingsInput struct {
	Zodiac      string       `json:"zodiac,omitempty"`
	Ayanamsha   string       `json:"ayanamsha,omitempty"`
	HouseSystem string       `json:"house_system,omitempty"`
	Bodies      []string     `json:"bodies,omitempty"`
	Topocentric bool         `json:"topocentric,omitempty"`
	Aspects     *AspectInput `json:"aspects,omitempty"`
}

// AspectInput lets a client choose which aspects to look for and widen or
// narrow the orbs.
type AspectInput struct {
	Types []string           `json:"types,omitempty"`
	Orbs  map[string]float64 `json:"orbs,omitempty"`
}

// maxOrbOverride caps how wide a client may set an orb. Beyond this every
// body aspects every other one and the result stops meaning anything.
const maxOrbOverride = 30.0

// Settings is the validated form of SettingsInput, with every default filled
// in and every name resolved to a catalogue entry.
type Settings struct {
	Zodiac      Zodiac
	Ayanamsha   Ayanamsha
	HouseSystem HouseSystem
	Bodies      []Body
	Topocentric bool
	AspectTypes []AspectType

	// Orbs is the effective base orb for every body, after any override.
	Orbs map[string]float64
}

// ResolveSettings validates a client's settings and fills in the defaults.
//
// Every name is checked against the catalogue rather than passed through, so a
// misspelling is reported instead of silently producing a chart cast with
// something other than what was asked for.
func ResolveSettings(in SettingsInput, loc Location) (Settings, error) {
	out := Settings{Topocentric: in.Topocentric}

	// Zodiac and ayanamsha.
	switch strings.ToLower(strings.TrimSpace(in.Zodiac)) {
	case "", string(ZodiacTropical):
		out.Zodiac = ZodiacTropical
	case string(ZodiacSidereal):
		out.Zodiac = ZodiacSidereal
	default:
		return Settings{}, &tz.FieldError{
			Field:   "settings.zodiac",
			Message: fmt.Sprintf("%q is not a zodiac; use tropical or sidereal", in.Zodiac),
		}
	}

	ayanamshaName := in.Ayanamsha
	if ayanamshaName == "" {
		ayanamshaName = DefaultAyanamsha
	}
	ayanamsha, ok := LookupAyanamsha(ayanamshaName)
	if !ok {
		return Settings{}, &tz.FieldError{
			Field: "settings.ayanamsha",
			Message: fmt.Sprintf("%q is not a known ayanamsha; see /v1/reference/ayanamshas",
				in.Ayanamsha),
		}
	}
	out.Ayanamsha = ayanamsha

	// House system.
	houseName := in.HouseSystem
	if houseName == "" {
		houseName = DefaultHouseSystem
	}
	house, ok := LookupHouseSystem(houseName)
	if !ok {
		return Settings{}, &tz.FieldError{
			Field: "settings.house_system",
			Message: fmt.Sprintf("%q is not a known house system; see /v1/reference/house-systems",
				in.HouseSystem),
		}
	}
	// Placidus and Koch are undefined near the poles. Refusing here is
	// better than returning cusps that are out of order.
	if err := loc.CheckHouseSystem(house); err != nil {
		return Settings{}, err
	}
	out.HouseSystem = house

	// Bodies.
	names := in.Bodies
	if len(names) == 0 {
		names = DefaultBodies()
	}
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		body, ok := LookupBody(name)
		if !ok {
			return Settings{}, &tz.FieldError{
				Field: "settings.bodies",
				Message: fmt.Sprintf("%q is not a known body; see /v1/reference/bodies",
					name),
			}
		}
		// An angle comes from the house calculation and is reported there,
		// so asking for one as a body is a category error worth naming.
		if body.Category == CategoryAngle {
			return Settings{}, &tz.FieldError{
				Field: "settings.bodies",
				Message: fmt.Sprintf("%q is a chart angle, not a body; angles are always "+
					"returned with the houses", name),
			}
		}
		if seen[body.Name] {
			continue
		}
		seen[body.Name] = true
		out.Bodies = append(out.Bodies, body)
	}

	// Aspects.
	aspectNames := DefaultAspects()
	if in.Aspects != nil && len(in.Aspects.Types) > 0 {
		aspectNames = in.Aspects.Types
	}
	seenAspect := make(map[string]bool, len(aspectNames))
	for _, name := range aspectNames {
		aspect, ok := LookupAspect(name)
		if !ok {
			return Settings{}, &tz.FieldError{
				Field: "settings.aspects.types",
				Message: fmt.Sprintf("%q is not a known aspect; see /v1/reference/aspects",
					name),
			}
		}
		if seenAspect[aspect.Name] {
			continue
		}
		seenAspect[aspect.Name] = true
		out.AspectTypes = append(out.AspectTypes, aspect)
	}

	// Orbs: start from the catalogue and apply any override.
	out.Orbs = make(map[string]float64, len(bodyIndex))
	for name, body := range bodyIndex {
		out.Orbs[name] = body.BaseOrb
	}
	if in.Aspects != nil {
		for name, orb := range in.Aspects.Orbs {
			body, ok := LookupBody(name)
			if !ok {
				return Settings{}, &tz.FieldError{
					Field: "settings.aspects.orbs",
					Message: fmt.Sprintf("%q is not a known body; see /v1/reference/bodies",
						name),
				}
			}
			if orb < 0 || orb > maxOrbOverride {
				return Settings{}, &tz.FieldError{
					Field: "settings.aspects.orbs",
					Message: fmt.Sprintf("the orb for %s must be between 0 and %g degrees, got %g",
						body.Name, maxOrbOverride, orb),
				}
			}
			out.Orbs[body.Name] = orb
		}
	}

	return out, nil
}

// BodyNames returns the requested body names, for the response meta.
func (s Settings) BodyNames() []string {
	out := make([]string, len(s.Bodies))
	for i, b := range s.Bodies {
		out[i] = b.Name
	}
	return out
}

// AspectNames returns the requested aspect names, sorted for a stable
// response.
func (s Settings) AspectNames() []string {
	out := make([]string, len(s.AspectTypes))
	for i, a := range s.AspectTypes {
		out[i] = a.Name
	}
	sort.Strings(out)
	return out
}

// sweOptions translates the settings into the options the calculation layer
// takes.
func (s Settings) sweOptions(loc Location) swe.Options {
	o := swe.Options{
		Topocentric: s.Topocentric,
		Latitude:    loc.Latitude,
		Longitude:   loc.Longitude,
		AltitudeM:   loc.AltitudeM,
	}
	if s.Zodiac == ZodiacSidereal {
		o.Sidereal = true
		o.Ayanamsha = s.Ayanamsha.SwissID
	}
	return o
}
