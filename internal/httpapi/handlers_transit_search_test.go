package httpapi

import (
	"encoding/json"
	"math"
	"net/http"
	"testing"
)

func TestTransitSearchEndpoint(t *testing.T) {
	s := newTestServer(t, nil)

	rec := postJSON(t, s, "/v1/transits/search", `{
		"natal": `+chartABody+`,
		"from": {"date": "2026-01-01", "time": "00:00:00Z"},
		"to": {"date": "2028-01-01", "time": "00:00:00Z"},
		"bodies": ["saturn"]
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}

	var got transitSearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("could not decode body: %v", err)
	}
	if len(got.Contacts) == 0 {
		t.Fatal("two years of Saturn against a full chart found nothing")
	}
	if got.Meta.Count != len(got.Contacts) {
		t.Errorf("meta says %d contacts, the list has %d", got.Meta.Count, len(got.Contacts))
	}

	// Earliest first, and each contact really is the aspect it claims.
	for i, c := range got.Contacts {
		if i > 0 && c.JulianDayUT < got.Contacts[i-1].JulianDayUT {
			t.Errorf("contact %d comes before contact %d", i, i-1)
		}
		sep := math.Abs(math.Mod(c.TransitingLongitude-c.NatalLongitude+540, 360) - 180)
		if math.Abs(sep-c.Angle) > 1e-6 {
			t.Errorf("%s %s %s: the two are %.6f degrees apart, want %g",
				c.Transiting, c.Aspect, c.Natal, sep, c.Angle)
		}
		if c.Pass < 1 || c.Pass > c.Passes {
			t.Errorf("pass %d of %d makes no sense", c.Pass, c.Passes)
		}
	}
}

// TestTransitSearchReportsTriplePasses checks that the retrograde loops reach
// the response, since they are the reason the endpoint is useful.
func TestTransitSearchReportsTriplePasses(t *testing.T) {
	s := newTestServer(t, nil)

	rec := postJSON(t, s, "/v1/transits/search", `{
		"natal": `+chartABody+`,
		"from": {"date": "2026-01-01", "time": "00:00:00Z"},
		"to": {"date": "2028-01-01", "time": "00:00:00Z"},
		"bodies": ["saturn"]
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}

	var got transitSearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("could not decode body: %v", err)
	}

	var retrograde, triples int
	for _, c := range got.Contacts {
		if c.Retrograde {
			retrograde++
		}
		if c.Passes == 3 {
			triples++
		}
	}
	if triples == 0 {
		t.Error("no triple contact was reported, although Saturn turned back twice in this span")
	}
	if retrograde == 0 {
		t.Error("no contact was marked retrograde, so the middle pass cannot be told from the others")
	}
}

func TestTransitSearchRejectsBadInput(t *testing.T) {
	s := newTestServer(t, nil)

	cases := []struct{ name, body, field string }{
		{
			"longer than may be searched", `{
				"natal": ` + chartABody + `,
				"from": {"date": "2000-01-01", "time": "00:00:00Z"},
				"to": {"date": "2030-01-01", "time": "00:00:00Z"}
			}`,
			"to",
		},
		{
			"an angle as a transiting body", `{
				"natal": ` + chartABody + `,
				"from": {"date": "2026-01-01", "time": "00:00:00Z"},
				"to": {"date": "2027-01-01", "time": "00:00:00Z"},
				"bodies": ["ascendant"]
			}`,
			"bodies",
		},
		{
			"a point that is not in this chart", `{
				"natal": ` + chartABody + `,
				"from": {"date": "2026-01-01", "time": "00:00:00Z"},
				"to": {"date": "2027-01-01", "time": "00:00:00Z"},
				"points": ["ceres"]
			}`,
			"points",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := postJSON(t, s, "/v1/transits/search", tc.body)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422; body: %s", rec.Code, rec.Body)
			}
			var p Problem
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatalf("could not decode problem: %v", err)
			}
			if len(p.Errors) == 0 || p.Errors[0].Field != tc.field {
				t.Errorf("errors = %+v, want the field %q named", p.Errors, tc.field)
			}
		})
	}
}
