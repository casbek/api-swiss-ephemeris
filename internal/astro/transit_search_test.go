package astro

import (
	"context"
	"math"
	"testing"

	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

func searchRequest(adjust func(*TransitSearchRequest)) TransitSearchRequest {
	req := TransitSearchRequest{
		Natal:        referenceRequest,
		RangeRequest: span("2026-01-01", "2028-01-01"),
	}
	if adjust != nil {
		adjust(&req)
	}
	return req
}

// TestSearchedContactsAreActuallyExact is the strongest check available: go to
// each moment the search names, cast the sky there, and measure the angle.
func TestSearchedContactsAreActuallyExact(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	result, err := e.SearchTransits(ctx, searchRequest(func(r *TransitSearchRequest) {
		r.Bodies = []string{"jupiter", "saturn"}
	}))
	if err != nil {
		t.Fatalf("SearchTransits failed: %v", err)
	}
	if len(result.Contacts) == 0 {
		t.Fatal("two years of Jupiter and Saturn against a full chart found nothing")
	}

	for _, c := range result.Contacts {
		at, err := e.Positions(ctx, PositionsRequest{
			DateTime: tz.Input{
				Date: c.ExactAt.Format("2006-01-02"),
				Time: c.ExactAt.Format("15:04:05") + "Z",
			},
			Settings: SettingsInput{Bodies: []string{c.Transiting}},
		})
		if err != nil {
			t.Fatalf("Positions failed: %v", err)
		}

		got := Separation(at.Positions[0].Longitude, c.NatalLongitude)
		// A second of rounding in the round trip through the calendar lets the
		// faster bodies drift a little.
		if math.Abs(got-c.Angle) > 0.01 {
			t.Errorf("%s %s %s is exact at %s, but the separation there is %.6f, want %g",
				c.Transiting, c.Aspect, c.Natal,
				c.ExactAt.Format("2006-01-02 15:04"), got, c.Angle)
		}
	}
}

// TestRetrogradeLoopsPerfectThreeTimes checks the thing that makes this
// endpoint worth having. A slow planet that turns back over an aspect perfects
// it three times, and a search that stopped at the first would miss the two
// that follow.
func TestRetrogradeLoopsPerfectThreeTimes(t *testing.T) {
	e := newTestEngine(t)

	result, err := e.SearchTransits(context.Background(), searchRequest(func(r *TransitSearchRequest) {
		r.Bodies = []string{"saturn"}
	}))
	if err != nil {
		t.Fatalf("SearchTransits failed: %v", err)
	}

	triples := 0
	for _, c := range result.Contacts {
		if c.Passes != 3 || c.Pass != 1 {
			continue
		}
		triples++

		// Collect the three and check the shape of the loop.
		var loop []TransitContact
		for _, other := range result.Contacts {
			if other.Transiting == c.Transiting && other.Natal == c.Natal && other.Aspect == c.Aspect {
				loop = append(loop, other)
			}
		}
		if len(loop) != 3 {
			t.Fatalf("%s %s %s claims three passes but %d were collected",
				c.Transiting, c.Aspect, c.Natal, len(loop))
		}

		// They must be in order, and numbered in that order.
		for i := 1; i < 3; i++ {
			if loop[i].JulianDayUT <= loop[i-1].JulianDayUT {
				t.Errorf("%s %s %s: pass %d is not after pass %d",
					c.Transiting, c.Aspect, c.Natal, i+1, i)
			}
			if loop[i].Pass != i+1 {
				t.Errorf("the passes are numbered %d, %d", loop[i-1].Pass, loop[i].Pass)
			}
		}

		// Direct, retrograde, direct. That is what a loop over an aspect is,
		// and it is the only way the same degree is reached three times.
		if loop[0].Retrograde || !loop[1].Retrograde || loop[2].Retrograde {
			t.Errorf("%s %s %s passes are retrograde %v, %v, %v; want direct, retrograde, direct",
				c.Transiting, c.Aspect, c.Natal,
				loop[0].Retrograde, loop[1].Retrograde, loop[2].Retrograde)
		}

		// A Saturn loop runs the better part of a year end to end.
		months := (loop[2].JulianDayUT - loop[0].JulianDayUT) / 30.44
		if months < 3 || months > 14 {
			t.Errorf("%s %s %s spans %.1f months, which is not a retrograde loop",
				c.Transiting, c.Aspect, c.Natal, months)
		}
	}

	if triples == 0 {
		t.Error("Saturn turned retrograde twice in these two years and must have " +
			"perfected something three times")
	}
}

// TestSearchAgreesWithTheTransitEndpoint checks the two ways of asking against
// each other: at a moment the search names, the other endpoint should report
// that same aspect as exact.
func TestSearchAgreesWithTheTransitEndpoint(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	result, err := e.SearchTransits(ctx, searchRequest(func(r *TransitSearchRequest) {
		r.Bodies = []string{"saturn"}
		r.Points = []string{"sun", "moon"}
	}))
	if err != nil {
		t.Fatalf("SearchTransits failed: %v", err)
	}
	if len(result.Contacts) == 0 {
		t.Skip("no Saturn contacts to the luminaries in this span")
	}

	c := result.Contacts[0]
	transits, err := e.Transits(ctx, TransitRequest{
		Natal: referenceRequest,
		Transit: MomentRequest{DateTime: tz.Input{
			Date: c.ExactAt.Format("2006-01-02"),
			Time: c.ExactAt.Format("15:04:05") + "Z",
		}},
	})
	if err != nil {
		t.Fatalf("Transits failed: %v", err)
	}

	var found bool
	for _, a := range transits.Aspects {
		if a.From == c.Transiting && a.To == c.Natal && a.Type == c.Aspect {
			found = true
			if a.Orb > 0.01 {
				t.Errorf("the search calls %s %s %s exact at %s, but the transit endpoint "+
					"gives it an orb of %.4f there",
					c.Transiting, c.Aspect, c.Natal, c.ExactAt.Format("2006-01-02 15:04"), a.Orb)
			}
		}
	}
	if !found {
		t.Errorf("the transit endpoint does not report %s %s %s at the moment the search "+
			"says it perfects", c.Transiting, c.Aspect, c.Natal)
	}
}

// TestSearchFindsBothSidesOfAnAspect checks that a body reaching an aspect from
// either direction is found. A trine has two exact degrees, one on each side of
// the natal point, and reporting only one would silently halve the answer.
func TestSearchFindsBothSidesOfAnAspect(t *testing.T) {
	e := newTestEngine(t)

	// A full circuit of the zodiac, so the body must pass both degrees.
	result, err := e.SearchTransits(context.Background(), TransitSearchRequest{
		Natal:        referenceRequest,
		RangeRequest: span("2026-01-01", "2028-06-01"),
		Bodies:       []string{"mars"},
		Points:       []string{"sun"},
		Aspects:      []string{"trine"},
	})
	if err != nil {
		t.Fatalf("SearchTransits failed: %v", err)
	}

	natalSun := positionOf(t, result.Natal, "sun").Longitude
	var ahead, behind bool
	for _, c := range result.Contacts {
		diff := NormalizeSigned(c.TransitingLongitude - natalSun)
		if math.Abs(diff-120) < 0.1 {
			ahead = true
		}
		if math.Abs(diff+120) < 0.1 {
			behind = true
		}
	}
	if !ahead || !behind {
		t.Errorf("over two and a half years Mars should trine the natal Sun from both "+
			"sides; found ahead=%v behind=%v", ahead, behind)
	}
}

// TestAspectTargetsAreNotDoubled checks the two aspects whose degrees coincide.
// A conjunction has one exact degree and an opposition's two are the same
// point, so searching either twice would report every contact twice.
func TestAspectTargetsAreNotDoubled(t *testing.T) {
	cases := []struct {
		angle float64
		want  int
	}{
		{0, 1},   // conjunction
		{180, 1}, // opposition: ahead and behind are the same point
		{120, 2}, // trine
		{90, 2},  // square
		{60, 2},  // sextile
	}
	for _, tc := range cases {
		if got := len(aspectTargets(100, tc.angle)); got != tc.want {
			t.Errorf("an aspect of %g degrees has %d exact degrees, want %d",
				tc.angle, got, tc.want)
		}
	}
}

// TestSearchDefaultsToTheSlowBodies checks the default set, and why it is what
// it is: with the fast bodies in, a year of contacts is several times longer
// and the slow ones are buried in it.
func TestSearchDefaultsToTheSlowBodies(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	slow, err := e.SearchTransits(ctx, TransitSearchRequest{
		Natal:        referenceRequest,
		RangeRequest: span("2026-01-01", "2027-01-01"),
	})
	if err != nil {
		t.Fatalf("SearchTransits failed: %v", err)
	}

	for _, name := range slow.Bodies {
		if name == "moon" || name == "sun" || name == "mercury" || name == "venus" {
			t.Errorf("%s is in the default set, but it contacts everything too often "+
				"to be listed by date", name)
		}
	}
	if len(slow.Contacts) == 0 {
		t.Fatal("a year of slow transits against a full chart found nothing")
	}

	withFast, err := e.SearchTransits(ctx, TransitSearchRequest{
		Natal:        referenceRequest,
		RangeRequest: span("2026-01-01", "2027-01-01"),
		Bodies:       []string{"sun", "mercury", "venus", "mars", "jupiter", "saturn"},
	})
	if err != nil {
		t.Fatalf("SearchTransits failed: %v", err)
	}
	if len(withFast.Contacts) <= len(slow.Contacts) {
		t.Errorf("adding the fast bodies gave %d contacts against %d without them, so "+
			"either they were not followed or the default is not doing its job",
			len(withFast.Contacts), len(slow.Contacts))
	}
}

func TestSearchRejectsBadInput(t *testing.T) {
	e := newTestEngine(t)

	cases := []struct {
		name  string
		req   TransitSearchRequest
		field string
	}{
		{
			"longer than may be searched",
			searchRequest(func(r *TransitSearchRequest) {
				r.RangeRequest = span("2000-01-01", "2030-01-01")
			}),
			"to",
		},
		{
			"an unknown transiting body",
			searchRequest(func(r *TransitSearchRequest) { r.Bodies = []string{"nibiru"} }),
			"bodies",
		},
		{
			"an angle as a transiting body",
			searchRequest(func(r *TransitSearchRequest) { r.Bodies = []string{"ascendant"} }),
			"bodies",
		},
		{
			"an unknown natal point",
			searchRequest(func(r *TransitSearchRequest) { r.Points = []string{"nibiru"} }),
			"points",
		},
		{
			"a point that is not in this chart",
			searchRequest(func(r *TransitSearchRequest) { r.Points = []string{"ceres"} }),
			"points",
		},
		{
			"an unknown aspect",
			searchRequest(func(r *TransitSearchRequest) { r.Aspects = []string{"octile"} }),
			"aspects",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.SearchTransits(context.Background(), tc.req)
			if err == nil {
				t.Fatal("expected an error")
			}
			fe, ok := err.(*tz.FieldError)
			if !ok {
				t.Fatalf("error is %T, want a *tz.FieldError: %v", err, err)
			}
			if fe.Field != tc.field {
				t.Errorf("field = %q, want %q (%v)", fe.Field, tc.field, err)
			}
		})
	}
}

// TestSearchAnswersTheQuestionItIsFor checks the narrow case the endpoint
// exists to answer: when will one planet make one aspect to one point.
func TestSearchAnswersTheQuestionItIsFor(t *testing.T) {
	e := newTestEngine(t)

	result, err := e.SearchTransits(context.Background(), TransitSearchRequest{
		Natal:        referenceRequest,
		RangeRequest: span("2026-01-01", "2032-01-01"),
		Bodies:       []string{"saturn"},
		Points:       []string{"moon"},
		Aspects:      []string{"square"},
	})
	if err != nil {
		t.Fatalf("SearchTransits failed: %v", err)
	}

	if len(result.Contacts) == 0 {
		t.Fatal("Saturn covers most of a circuit in six years and must square the " +
			"natal Moon somewhere in it")
	}
	for _, c := range result.Contacts {
		if c.Transiting != "saturn" || c.Natal != "moon" || c.Aspect != "square" {
			t.Errorf("asked for Saturn square Moon, got %s %s %s",
				c.Transiting, c.Aspect, c.Natal)
		}
		if got := Separation(c.TransitingLongitude, c.NatalLongitude); math.Abs(got-90) > 1e-6 {
			t.Errorf("the contact is %.6f degrees from the natal Moon, want 90", got)
		}
	}
}
