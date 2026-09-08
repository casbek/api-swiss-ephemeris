package httpapi

import (
	"encoding/json"
	"math"
	"net/http"
	"testing"
)

const vedicNatal = `"natal": {
	"datetime": {"date": "1990-06-15", "time": "17:30:00", "timezone": "Europe/Istanbul"},
	"location": {"latitude": 41.0082, "longitude": 28.9784}
}`

func TestDashasEndpoint(t *testing.T) {
	s := newTestServer(t, nil)

	rec := postJSON(t, s, "/v1/vedic/dashas", `{`+vedicNatal+`, "depth": 2}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}

	var got dashaResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("could not decode body: %v", err)
	}

	if len(got.Periods) != 9 {
		t.Fatalf("got %d great periods, want one per lord", len(got.Periods))
	}
	// Asking for a dasha is asking for the sidereal zodiac, whether or not
	// the request said so.
	if got.Meta.Zodiac != "sidereal" {
		t.Errorf("zodiac = %q, want sidereal", got.Meta.Zodiac)
	}
	// The convention that decides the dates has to be reported, since it is
	// the usual reason two services disagree about them.
	if got.Meta.YearLength != "julian" || got.Meta.YearLengthDays != 365.25 {
		t.Errorf("year length = %q (%g days), want julian at 365.25",
			got.Meta.YearLength, got.Meta.YearLengthDays)
	}
	if got.Meta.StartingLord != got.Meta.Nakshatra.Lord {
		t.Errorf("the sequence opens with %s but the Moon's mansion is ruled by %s",
			got.Meta.StartingLord, got.Meta.Nakshatra.Lord)
	}
	if got.Meta.Nakshatra.Pada < 1 || got.Meta.Nakshatra.Pada > 4 {
		t.Errorf("pada = %d", got.Meta.Nakshatra.Pada)
	}
	if len(got.Periods[0].Periods) == 0 {
		t.Error("depth 2 was asked for but the great periods carry no divisions")
	}

	// The Moon in the natal half must carry the mansion the dasha was read
	// from, since the chart is sidereal.
	for _, b := range got.Natal.Bodies {
		if b.Body != "moon" {
			continue
		}
		if b.Nakshatra == nil {
			t.Fatal("the Moon carries no nakshatra in a sidereal chart")
		}
		if b.Nakshatra.Number != got.Meta.Nakshatra.Number {
			t.Errorf("the Moon is in mansion %d but the dasha was read from %d",
				b.Nakshatra.Number, got.Meta.Nakshatra.Number)
		}
	}
}

func TestDashasEndpointRejectsTropical(t *testing.T) {
	s := newTestServer(t, nil)

	rec := postJSON(t, s, "/v1/vedic/dashas", `{
		"natal": {
			"datetime": {"date": "1990-06-15", "time": "17:30:00", "timezone": "Europe/Istanbul"},
			"location": {"latitude": 41.0082, "longitude": 28.9784},
			"settings": {"zodiac": "tropical"}
		}
	}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body: %s", rec.Code, rec.Body)
	}

	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("could not decode problem: %v", err)
	}
	if len(p.Errors) == 0 || p.Errors[0].Field != "natal.settings.zodiac" {
		t.Errorf("errors = %+v, want the zodiac named", p.Errors)
	}
}

func TestDivisionalEndpoint(t *testing.T) {
	s := newTestServer(t, nil)

	rec := postJSON(t, s, "/v1/vedic/divisional",
		`{`+vedicNatal+`, "charts": ["d1", "d9", "d30"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}

	var got divisionalResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("could not decode body: %v", err)
	}
	if len(got.Charts) != 3 {
		t.Fatalf("got %d charts, want 3", len(got.Charts))
	}
	if got.Meta.Zodiac != "sidereal" {
		t.Errorf("zodiac = %q, want sidereal", got.Meta.Zodiac)
	}

	for _, c := range got.Charts {
		if len(c.Bodies) == 0 {
			t.Errorf("%s holds no bodies", c.Name)
		}
		if c.Ascendant == nil {
			t.Errorf("%s has no ascendant, although the birth time is known", c.Name)
		}
		for _, b := range c.Bodies {
			if b.SignIndex < 0 || b.SignIndex > 11 {
				t.Errorf("%s puts %s in sign index %d", c.Name, b.Body, b.SignIndex)
			}
			if b.Division < 1 || b.Division > c.Divisions {
				t.Errorf("%s puts %s in division %d of %d",
					c.Name, b.Body, b.Division, c.Divisions)
			}
		}
	}

	// The trimshamsha is the one division with unequal parts, so its division
	// number counts the five stretches rather than thirty slices.
	for _, c := range got.Charts {
		if c.Name != "d30" {
			continue
		}
		for _, b := range c.Bodies {
			if b.Division > 5 {
				t.Errorf("the trimshamsha puts %s in part %d, but it has five stretches",
					b.Body, b.Division)
			}
		}
	}
}

// TestDivisionalRashiIsTheChartItself checks that the first of the sixteen is
// no division at all.
func TestDivisionalRashiIsTheChartItself(t *testing.T) {
	s := newTestServer(t, nil)

	rec := postJSON(t, s, "/v1/vedic/divisional", `{`+vedicNatal+`, "charts": ["d1"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}

	var got divisionalResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("could not decode body: %v", err)
	}

	type placement struct {
		sign      int
		longitude float64
	}
	natal := map[string]placement{}
	for _, b := range got.Natal.Bodies {
		natal[b.Body] = placement{b.SignIndex, b.Longitude}
	}

	for _, b := range got.Charts[0].Bodies {
		want := natal[b.Body]
		if b.SignIndex != want.sign {
			t.Errorf("the rashi chart moved %s from sign %d to %d",
				b.Body, want.sign, b.SignIndex)
		}
		if math.Abs(b.RashiLongitude-want.longitude) > 1e-9 {
			t.Errorf("%s reports longitude %.6f, want the natal %.6f",
				b.Body, b.RashiLongitude, want.longitude)
		}
	}
}

func TestVedicReferenceTopics(t *testing.T) {
	s := newTestServer(t, nil)

	for _, topic := range []string{"nakshatras", "divisional-charts"} {
		rec := do(t, s, http.MethodGet, "/v1/reference/"+topic, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", topic, rec.Code)
			continue
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Errorf("%s: could not decode body: %v", topic, err)
		}
		if len(body) == 0 {
			t.Errorf("%s: body is empty", topic)
		}
	}
}
