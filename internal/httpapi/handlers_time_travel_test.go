package httpapi

import (
	"encoding/json"
	"math"
	"net/http"
	"testing"
)

func TestReturnsEndpoint(t *testing.T) {
	s := newTestServer(t, nil)

	rec := postJSON(t, s, "/v1/returns", `{
		"natal": `+chartABody+`,
		"body": "sun",
		"from": {"date": "2024-01-01", "time": "00:00:00Z"}
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}

	var got returnsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("could not decode body: %v", err)
	}
	if len(got.Returns) != 1 {
		t.Fatalf("got %d returns, want 1", len(got.Returns))
	}
	if got.Meta.SearchedFrom == "" {
		t.Error("the response does not say where the search started")
	}

	entry := got.Returns[0]
	// The solar return is the anniversary of the birth, give or take a day.
	if entry.ExactUTC[:7] != "2024-06" {
		t.Errorf("the return is at %s, want June 2024", entry.ExactUTC)
	}

	// At that moment the Sun stands on the degree it held at birth. That is
	// the whole definition, and it is checkable from the response alone.
	var sun float64
	for _, b := range entry.Chart.Bodies {
		if b.Body == "sun" {
			sun = b.Longitude
		}
	}
	if diff := math.Abs(sun - entry.NatalLongitude); diff > 1e-5 {
		t.Errorf("the Sun is %.8f degrees off the degree it returned to", diff)
	}
}

func TestReturnsEndpointCount(t *testing.T) {
	s := newTestServer(t, nil)

	rec := postJSON(t, s, "/v1/returns", `{
		"natal": `+chartABody+`,
		"body": "moon",
		"from": {"date": "2024-01-01", "time": "00:00:00Z"},
		"count": 4
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}

	var got returnsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("could not decode body: %v", err)
	}
	if len(got.Returns) != 4 {
		t.Fatalf("got %d returns, want 4", len(got.Returns))
	}
	for i := 1; i < len(got.Returns); i++ {
		if got.Returns[i].ExactUTC <= got.Returns[i-1].ExactUTC {
			t.Errorf("return %d is not after return %d", i, i-1)
		}
	}
}

func TestReturnsEndpointRejectsBadInput(t *testing.T) {
	s := newTestServer(t, nil)

	cases := []struct{ name, body, field string }{
		{
			"a body with no return chart",
			`{"natal": ` + chartABody + `, "body": "mars", "from": {"date": "2024-01-01", "time": "00:00Z"}}`,
			"body",
		},
		{
			"more returns than are sensible",
			`{"natal": ` + chartABody + `, "body": "moon", "count": 500,
			  "from": {"date": "2024-01-01", "time": "00:00Z"}}`,
			"count",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := postJSON(t, s, "/v1/returns", tc.body)
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

func TestProgressionsEndpoint(t *testing.T) {
	s := newTestServer(t, nil)

	t.Run("secondary is cast for a moment", func(t *testing.T) {
		rec := postJSON(t, s, "/v1/progressions", `{
			"natal": `+chartABody+`,
			"target": {"datetime": {"date": "2024-06-15", "time": "12:00:00Z"}},
			"method": "secondary"
		}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
		}

		var got progressionResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("could not decode body: %v", err)
		}
		if got.Method != "secondary" {
			t.Errorf("method = %q", got.Method)
		}
		if math.Abs(got.Meta.ElapsedYears-34) > 0.02 {
			t.Errorf("elapsed_years = %.4f, want about 34", got.Meta.ElapsedYears)
		}
		// Thirty four years of life is thirty four days of ephemeris, so the
		// chart is cast for July 1990.
		if got.Meta.ProgressedUTC[:7] != "1990-07" {
			t.Errorf("progressed_utc = %s, want July 1990", got.Meta.ProgressedUTC)
		}
		if got.Meta.ArcDegrees != 0 {
			t.Error("a secondary progression reported an arc, which belongs to the other method")
		}
	})

	t.Run("solar arc reports its arc and no moment", func(t *testing.T) {
		rec := postJSON(t, s, "/v1/progressions", `{
			"natal": `+chartABody+`,
			"target": {"datetime": {"date": "2024-06-15", "time": "12:00:00Z"}},
			"method": "solar_arc"
		}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
		}

		var got progressionResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("could not decode body: %v", err)
		}
		if got.Meta.ArcDegrees < 32 || got.Meta.ArcDegrees > 36 {
			t.Errorf("arc_degrees = %.4f, want about 34", got.Meta.ArcDegrees)
		}
		if got.Meta.ProgressedUTC != "" {
			t.Errorf("a solar arc reported the moment %q, but it is cast for none",
				got.Meta.ProgressedUTC)
		}
		if len(got.Notes) == 0 {
			t.Error("nothing explains that the directed chart keeps its natal aspects")
		}
	})

	t.Run("secondary is the default", func(t *testing.T) {
		rec := postJSON(t, s, "/v1/progressions", `{
			"natal": `+chartABody+`,
			"target": {"datetime": {"date": "2024-06-15", "time": "12:00:00Z"}}
		}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
		}
		var got progressionResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("could not decode body: %v", err)
		}
		if got.Method != "secondary" {
			t.Errorf("method = %q, want secondary by default", got.Method)
		}
	})
}

// TestTransitsCarryExactTimes checks that the endpoint reports when each
// transit perfects.
func TestTransitsCarryExactTimes(t *testing.T) {
	s := newTestServer(t, nil)

	rec := postJSON(t, s, "/v1/transits", `{
		"natal": `+chartABody+`,
		"transit": {"datetime": {"date": "2024-03-15", "time": "12:00:00", "timezone": "Europe/Istanbul"}}
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}

	var got transitResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("could not decode body: %v", err)
	}

	withExact := 0
	for _, a := range got.Aspects {
		if a.ExactAt == nil {
			continue
		}
		withExact++
		// The moment must be near the transit, not somewhere in antiquity.
		if a.ExactAt.Year() < 2020 || a.ExactAt.Year() > 2030 {
			t.Errorf("%s %s %s perfects at %v, which is nowhere near the transit",
				a.From, a.Type, a.To, a.ExactAt)
		}
	}
	if withExact == 0 {
		t.Error("no transit reported when it perfects")
	}
}
