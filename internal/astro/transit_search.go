package astro

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/casbek/api-swiss-ephemeris/internal/swe"
	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// Searching a span for transits asks the same question as /v1/transits from the
// other end: not "what is touching this chart now" but "when will it".
//
// The answer is a list of moments, and finding them is the whole difficulty.
// A slow planet that turns retrograde over an aspect perfects it three times,
// months apart, and those three dates are exactly what makes the question worth
// asking. A search that stopped at the first would miss the other two.

const (
	// maxTransitSearchYears bounds the span. The cost grows with it, and the
	// question this endpoint answers is nearly always about the next year or
	// two.
	maxTransitSearchYears = 10

	// maxTransitContacts bounds the answer. A list longer than this is not
	// something a reader can use, and reaching it means the request was too
	// broad rather than the sky unusually busy.
	maxTransitContacts = 1000

	// transitSampleDays is how finely the sky is sampled while looking for
	// crossings. A day is short enough for every body: the Moon covers
	// thirteen degrees in one, far less than the half circle that would be
	// needed to step over a crossing unseen.
	transitSampleDays = 1.0
)

// TransitSearchRequest asks when the transiting bodies will contact a chart.
type TransitSearchRequest struct {
	Natal ChartRequest `json:"natal"`

	RangeRequest

	// Bodies are the transiting bodies to follow. It defaults to Mars and
	// everything slower; see defaultTransitingBodies.
	Bodies []string `json:"bodies,omitempty"`

	// Points are the natal points to watch. It defaults to everything in the
	// chart, the angles included.
	Points []string `json:"points,omitempty"`

	// Aspects are the contacts to look for, defaulting to the major five.
	Aspects []string `json:"aspects,omitempty"`

	Settings *SettingsInput `json:"settings,omitempty"`
}

// TransitContact is one moment a transiting body perfects an aspect to a natal
// point.
type TransitContact struct {
	// Transiting names the moving body and Natal the point it contacts.
	Transiting string
	Natal      string

	Aspect string
	Angle  float64

	ExactAt     time.Time
	JulianDayUT float64

	// TransitingLongitude is where the moving body stood at the contact, and
	// NatalLongitude where the natal point stands. They differ by the angle.
	TransitingLongitude float64
	NatalLongitude      float64

	// Retrograde is whether the transiting body was moving backwards. It is
	// what distinguishes the middle pass of a triple contact from the two
	// either side of it.
	Retrograde bool

	// Pass numbers the contacts of this body, point and aspect within the span
	// searched, and Passes counts them. A body that turns retrograde over an
	// aspect perfects it three times; one whose loop begins or ends outside
	// the span shows fewer, since only what was searched can be counted.
	Pass   int
	Passes int
}

// TransitSearchResult is the chart, the span and what was found.
type TransitSearchResult struct {
	Natal    *Chart
	From     Moment
	To       Moment
	Settings Settings

	// Bodies and Points are what was actually followed, after the defaults
	// were filled in.
	Bodies []string
	Points []string

	Contacts []TransitContact

	// Truncated is set when the search stopped at the limit rather than at the
	// end of the span. The contacts returned are still the earliest ones, but
	// there are more.
	Truncated bool
}

// SearchTransits finds when the transiting bodies perfect aspects to a chart.
func (e *Engine) SearchTransits(ctx context.Context, req TransitSearchRequest) (*TransitSearchResult, error) {
	natal, err := e.Cast(ctx, req.Natal)
	if err != nil {
		return nil, prefixField(err, "natal")
	}

	from, to, err := resolveRangeWithin(req.RangeRequest, maxTransitSearchYears)
	if err != nil {
		return nil, err
	}

	settings := natal.Settings
	if req.Settings != nil {
		if settings, err = ResolveSettings(*req.Settings, req.Natal.Location); err != nil {
			return nil, err
		}
	}

	transiting, err := transitingBodies(req.Bodies)
	if err != nil {
		return nil, err
	}
	targets, err := natalPoints(req.Points, natal)
	if err != nil {
		return nil, err
	}
	aspects, err := searchAspects(req.Aspects, settings)
	if err != nil {
		return nil, err
	}

	result := &TransitSearchResult{
		Natal:    natal,
		From:     from,
		To:       to,
		Settings: settings,
		Bodies:   bodyNames(transiting),
		Points:   positionNames(targets),
	}

	opts := settings.sweOptions(req.Natal.Location)

	err = e.calc.Do(ctx, func(s *swe.Session) error {
		for _, body := range transiting {
			read, ok := e.longitudeFunc(s, body.Name, opts)
			if !ok {
				continue
			}

			// The sky is sampled once per body and then read many times.
			// Searching each body, point and aspect separately would ask the
			// ephemeris for the same positions over and over: with a dozen
			// bodies against a dozen points and five aspects that is two
			// orders of magnitude more work for the same answer.
			track, err := sampleTrack(read, from.JulianDayUT, to.JulianDayUT)
			if err != nil {
				return err
			}

			for _, target := range targets {
				for _, aspect := range aspects {
					for _, degree := range aspectTargets(target.Longitude, aspect.Angle) {
						hits, err := track.crossings(read, degree)
						if err != nil {
							return err
						}
						for _, hit := range hits {
							result.Contacts = append(result.Contacts, TransitContact{
								Transiting:          body.Name,
								Natal:               target.Body,
								Aspect:              aspect.Name,
								Angle:               aspect.Angle,
								JulianDayUT:         hit.jd,
								TransitingLongitude: hit.longitude,
								NatalLongitude:      target.Longitude,
								Retrograde:          hit.speed < 0,
							})
						}
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(result.Contacts, func(i, j int) bool {
		if result.Contacts[i].JulianDayUT != result.Contacts[j].JulianDayUT {
			return result.Contacts[i].JulianDayUT < result.Contacts[j].JulianDayUT
		}
		return result.Contacts[i].Transiting < result.Contacts[j].Transiting
	})

	if len(result.Contacts) > maxTransitContacts {
		result.Contacts = result.Contacts[:maxTransitContacts]
		result.Truncated = true
	}

	numberPasses(result.Contacts)

	for i := range result.Contacts {
		moment, err := MomentFromJD(result.Contacts[i].JulianDayUT)
		if err != nil {
			return nil, err
		}
		result.Contacts[i].ExactAt = moment.UTC
	}
	return result, nil
}

// sample is the sky at one moment along a body's track.
type sample struct {
	jd        float64
	longitude float64
	speed     float64
}

// track is a body's path across the span, sampled at regular intervals.
type track []sample

// sampleTrack walks a body across the span once.
func sampleTrack(read longitudeFunc, from, to float64) (track, error) {
	out := make(track, 0, int((to-from)/transitSampleDays)+2)

	for jd := from; jd < to; jd += transitSampleDays {
		lon, speed, err := read(jd)
		if err != nil {
			return nil, err
		}
		out = append(out, sample{jd: jd, longitude: lon, speed: speed})
	}

	// The end of the span is a sample of its own, so a crossing in the last
	// partial step is not lost.
	lon, speed, err := read(to)
	if err != nil {
		return nil, err
	}
	return append(out, sample{jd: to, longitude: lon, speed: speed}), nil
}

// crossings finds every moment the body reaches a degree along this track.
//
// Every bracket is reported, not just the first. A body that turns retrograde
// over the degree crosses it three times, and those three dates are the reason
// the question was asked.
func (t track) crossings(read longitudeFunc, degree float64) ([]sample, error) {
	var found []sample

	for i := 1; i < len(t); i++ {
		prev := NormalizeSigned(t[i-1].longitude - degree)
		cur := NormalizeSigned(t[i].longitude - degree)

		// A sign change brackets a crossing only when both readings are near
		// the degree. The jump from +179 to -179 is the body passing the point
		// opposite it, which is not a crossing of this one.
		if !signsDiffer(prev, cur) || math.Abs(prev) >= 90 || math.Abs(cur) >= 90 {
			continue
		}

		jd, err := bisect(read, degree, t[i-1].jd, t[i].jd)
		if err != nil {
			return nil, err
		}
		lon, speed, err := read(jd)
		if err != nil {
			return nil, err
		}
		found = append(found, sample{jd: jd, longitude: lon, speed: speed})
	}
	return found, nil
}

// aspectTargets gives the degrees a body must reach to perfect an aspect to a
// point: one either side of it, or one when the two coincide.
func aspectTargets(natalLongitude, angle float64) []float64 {
	ahead := Normalize(natalLongitude + angle)
	behind := Normalize(natalLongitude - angle)

	// A conjunction has one degree, and an opposition's two are the same
	// point. Searching either twice would report every contact twice.
	if math.Abs(NormalizeSigned(ahead-behind)) < 1e-9 {
		return []float64{ahead}
	}
	return []float64{ahead, behind}
}

// numberPasses counts the contacts of each body, point and aspect, so a
// retrograde loop reads as the three passes it is rather than three unrelated
// dates.
func numberPasses(contacts []TransitContact) {
	type key struct{ transiting, natal, aspect string }

	total := map[key]int{}
	for _, c := range contacts {
		total[key{c.Transiting, c.Natal, c.Aspect}]++
	}

	seen := map[key]int{}
	for i := range contacts {
		k := key{contacts[i].Transiting, contacts[i].Natal, contacts[i].Aspect}
		seen[k]++
		contacts[i].Pass = seen[k]
		contacts[i].Passes = total[k]
	}
}

// defaultTransitingBodies is Mars and everything slower.
//
// The faster bodies are left out, and the reason is what the answer looks like
// with them in. Over a single year against a full chart they produce about six
// hundred and fifty contacts, against a hundred and fifty without them: the
// Moon alone reaches every point of every chart some thirteen times a year, and
// the Sun, Mercury and Venus each several. Those contacts are real, but they
// last hours and they bury the ones that last months, which are what a list of
// dates is useful for. A client that wants them need only ask.
var defaultTransitingBodies = []string{
	"mars", "jupiter", "saturn", "uranus", "neptune", "pluto", "chiron",
}

// transitingBodies resolves which bodies to follow.
func transitingBodies(names []string) ([]Body, error) {
	if len(names) == 0 {
		names = defaultTransitingBodies
	}

	out := make([]Body, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		body, ok := LookupBody(name)
		if !ok {
			return nil, &tz.FieldError{
				Field:   "bodies",
				Message: fmt.Sprintf("%q is not a known body; see /v1/reference/bodies", name),
			}
		}
		if body.Category == CategoryAngle {
			return nil, &tz.FieldError{
				Field: "bodies",
				Message: fmt.Sprintf("%q is a chart angle, which turns with the sky rather "+
					"than moving through it; it cannot be a transiting body", name),
			}
		}
		if seen[body.Name] {
			continue
		}
		seen[body.Name] = true
		out = append(out, body)
	}
	return out, nil
}

// natalPoints resolves which points of the chart to watch.
func natalPoints(names []string, natal *Chart) ([]Position, error) {
	available := natalTargets(natal)
	if len(names) == 0 {
		return available, nil
	}

	index := make(map[string]Position, len(available))
	for _, p := range available {
		index[p.Body] = p
	}

	out := make([]Position, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		p, ok := index[name]
		if !ok {
			// Naming the difference matters: an angle is missing from a chart
			// with an unknown birth time, which is a different problem from a
			// misspelling.
			if _, known := LookupBody(name); known {
				return nil, &tz.FieldError{
					Field: "points",
					Message: fmt.Sprintf("%q is not in this chart; it was not among the "+
						"bodies requested, or it is an angle and the birth time is unknown", name),
				}
			}
			return nil, &tz.FieldError{
				Field:   "points",
				Message: fmt.Sprintf("%q is not a known point; see /v1/reference/bodies", name),
			}
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, p)
	}
	return out, nil
}

// searchAspects resolves which contacts to look for.
func searchAspects(names []string, settings Settings) ([]AspectType, error) {
	if len(names) == 0 {
		return settings.AspectTypes, nil
	}

	out := make([]AspectType, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		aspect, ok := LookupAspect(name)
		if !ok {
			return nil, &tz.FieldError{
				Field:   "aspects",
				Message: fmt.Sprintf("%q is not a known aspect; see /v1/reference/aspects", name),
			}
		}
		if seen[aspect.Name] {
			continue
		}
		seen[aspect.Name] = true
		out = append(out, aspect)
	}
	return out, nil
}

func bodyNames(bodies []Body) []string {
	out := make([]string, len(bodies))
	for i, b := range bodies {
		out[i] = b.Name
	}
	return out
}

func positionNames(positions []Position) []string {
	out := make([]string, len(positions))
	for i, p := range positions {
		out[i] = p.Body
	}
	return out
}
