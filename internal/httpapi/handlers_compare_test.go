package httpapi

import (
	"encoding/json"
	"math"
	"net/http"
	"testing"
)

const (
	chartABody = `{"datetime":{"date":"1990-06-15","time":"17:30:00","timezone":"Europe/Istanbul"},
	               "location":{"latitude":41.0082,"longitude":28.9784}}`
	chartBBody = `{"datetime":{"date":"1985-11-22","time":"06:15:00","timezone":"Europe/Berlin"},
	               "location":{"latitude":52.52,"longitude":13.405}}`
)

func TestTransitsEndpoint(t *testing.T) {
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

	if got.Natal.Meta.UTC[:4] != "1990" {
		t.Errorf("the natal chart is for %s, want 1990", got.Natal.Meta.UTC)
	}
	if got.Transit.Meta.UTC[:4] != "2024" {
		t.Errorf("the transit chart is for %s, want 2024", got.Transit.Meta.UTC)
	}
	if len(got.Aspects) == 0 {
		t.Error("no transits were reported")
	}

	// From is always the transiting side and To always the natal one. Mixing
	// them up would invert every reading.
	transitBodies := map[string]bool{}
	for _, b := range got.Transit.Bodies {
		transitBodies[b.Body] = true
	}
	for _, a := range got.Aspects {
		if !transitBodies[a.From] {
			t.Errorf("%s is not a transiting body but appears as the source", a.From)
		}
	}
}

func TestSynastryEndpoint(t *testing.T) {
	s := newTestServer(t, nil)

	rec := postJSON(t, s, "/v1/synastry", `{"chart_a": `+chartABody+`, "chart_b": `+chartBBody+`}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}

	var got synastryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("could not decode body: %v", err)
	}
	if got.ChartA.Meta.Zone.Name != "Europe/Istanbul" {
		t.Errorf("chart_a zone = %q", got.ChartA.Meta.Zone.Name)
	}
	if got.ChartB.Meta.Zone.Name != "Europe/Berlin" {
		t.Errorf("chart_b zone = %q", got.ChartB.Meta.Zone.Name)
	}
	if len(got.Aspects) == 0 {
		t.Error("no contacts were reported")
	}
}

func TestCompositeEndpoint(t *testing.T) {
	s := newTestServer(t, nil)

	t.Run("davison is cast for a real moment", func(t *testing.T) {
		rec := postJSON(t, s, "/v1/composite",
			`{"chart_a": `+chartABody+`, "chart_b": `+chartBBody+`, "method": "davison"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
		}

		var got compositeResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("could not decode body: %v", err)
		}
		if got.Method != "davison" {
			t.Errorf("method = %q", got.Method)
		}
		// Halfway between November 1985 and June 1990.
		if got.Meta.UTC[:7] != "1988-03" {
			t.Errorf("the Davison moment is %s, want March 1988", got.Meta.UTC)
		}
		if got.Meta.Location == nil {
			t.Fatal("a Davison chart has a place and should report it")
		}
		// Between Istanbul at 41 north and Berlin at 52.5.
		if lat := got.Meta.Location.Latitude; math.Abs(lat-46.764) > 0.01 {
			t.Errorf("latitude = %.4f, want 46.764", lat)
		}
	})

	t.Run("midpoint has no moment", func(t *testing.T) {
		rec := postJSON(t, s, "/v1/composite",
			`{"chart_a": `+chartABody+`, "chart_b": `+chartBBody+`, "method": "midpoint"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
		}

		var got compositeResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("could not decode body: %v", err)
		}
		if got.Meta.UTC != "" {
			t.Errorf("a midpoint composite reported the moment %q, but it is a chart of none", got.Meta.UTC)
		}
		if len(got.Notes) == 0 {
			t.Error("nothing explains what a midpoint composite is not")
		}
	})

	t.Run("midpoint is the default", func(t *testing.T) {
		rec := postJSON(t, s, "/v1/composite",
			`{"chart_a": `+chartABody+`, "chart_b": `+chartBBody+`}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
		}
		var got compositeResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("could not decode body: %v", err)
		}
		if got.Method != "midpoint" {
			t.Errorf("method = %q, want midpoint by default", got.Method)
		}
	})

	t.Run("unknown method is refused", func(t *testing.T) {
		rec := postJSON(t, s, "/v1/composite",
			`{"chart_a": `+chartABody+`, "chart_b": `+chartBBody+`, "method": "harmonic"}`)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422", rec.Code)
		}
		var p Problem
		if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
			t.Fatalf("could not decode problem: %v", err)
		}
		if len(p.Errors) == 0 || p.Errors[0].Field != "method" {
			t.Errorf("errors = %+v, want the method named", p.Errors)
		}
	})
}

// TestComparisonErrorsNameTheChart checks that a client is told which half of
// the request to correct, rather than being left to guess.
func TestComparisonErrorsNameTheChart(t *testing.T) {
	s := newTestServer(t, nil)

	badChart := `{"datetime":{"date":"1990-06-15","time":"17:30","timezone":"Europe/Atlantis"},
	              "location":{"latitude":41,"longitude":29}}`

	cases := []struct{ name, path, body, field string }{
		{
			"synastry, second chart", "/v1/synastry",
			`{"chart_a": ` + chartABody + `, "chart_b": ` + badChart + `}`,
			"chart_b.datetime.timezone",
		},
		{
			"transits, transit half", "/v1/transits",
			`{"natal": ` + chartABody + `,
			  "transit": {"datetime": {"date": "2024-01-01", "time": "12:00", "timezone": "Europe/Atlantis"}}}`,
			"transit.datetime.timezone",
		},
		{
			"composite, first chart", "/v1/composite",
			`{"chart_a": ` + badChart + `, "chart_b": ` + chartBBody + `}`,
			"chart_a.datetime.timezone",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := postJSON(t, s, tc.path, tc.body)
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
