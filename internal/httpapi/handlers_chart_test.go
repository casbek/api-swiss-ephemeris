package httpapi

import (
	"encoding/json"
	"math"
	"net/http"
	"testing"
)

const referenceNatalBody = `{
	"datetime": {"date": "1990-06-15", "time": "17:30:00", "timezone": "Europe/Istanbul"},
	"location": {"latitude": 41.0082, "longitude": 28.9784}
}`

func decodeNatal(t *testing.T, s *Server, body string) natalResponse {
	t.Helper()
	rec := postJSON(t, s, "/v1/natal", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	var out natalResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("could not decode body: %v", err)
	}
	return out
}

func TestNatalChart(t *testing.T) {
	s := newTestServer(t, nil)
	got := decodeNatal(t, s, referenceNatalBody)

	if len(got.Bodies) != 13 {
		t.Errorf("got %d bodies, want the 13 of the default set", len(got.Bodies))
	}
	if got.Houses == nil || len(got.Houses.Cusps) != 12 {
		t.Fatal("the chart has no houses")
	}
	if got.Sect != "day" {
		t.Errorf("sect = %q, want day", got.Sect)
	}

	sun := got.Bodies[0]
	if sun.Body != "sun" {
		t.Fatalf("the first body is %s, want the Sun", sun.Body)
	}
	if diff := math.Abs(sun.Longitude - 84.2290447); diff > 1e-5 {
		t.Errorf("the Sun is at %.7f, want 84.2290447", sun.Longitude)
	}
	if sun.Formatted != `24°13'45" Gemini` {
		t.Errorf("formatted = %q", sun.Formatted)
	}
	if sun.House != 8 {
		t.Errorf("the Sun is in house %d, want 8", sun.House)
	}

	// The meta block has to carry everything that could change the result,
	// so a client comparing with another service can see the settings.
	if got.Meta.Zodiac != "tropical" {
		t.Errorf("zodiac = %q", got.Meta.Zodiac)
	}
	if got.Meta.HouseSystem != "placidus" {
		t.Errorf("house_system = %q", got.Meta.HouseSystem)
	}
	if got.Meta.Zone.Name != "Europe/Istanbul" || got.Meta.Zone.Offset != "+03:00" {
		t.Errorf("timezone = %+v", got.Meta.Zone)
	}
	if got.Meta.Source == "" {
		t.Error("the meta block does not point at the source")
	}
	if got.Meta.Ayanamsha != "" {
		t.Error("an ayanamsha is reported for a tropical chart, where it plays no part")
	}
}

func TestNatalWithoutTimeOmitsHouses(t *testing.T) {
	s := newTestServer(t, nil)
	got := decodeNatal(t, s, `{
		"datetime": {"date": "1990-06-15", "timezone": "Europe/Istanbul"},
		"location": {"latitude": 41.0082, "longitude": 28.9784}
	}`)

	if got.Houses != nil {
		t.Error("houses were returned for a chart with an unknown birth time")
	}
	if got.Meta.HouseSystem != "" {
		t.Error("a house system is reported although no houses were calculated")
	}
	if got.Meta.TimeKnown {
		t.Error("time_known is true although no time was sent")
	}
	if len(got.Notes) == 0 {
		t.Error("nothing tells the client that the time was assumed")
	}
}

func TestNatalSidereal(t *testing.T) {
	s := newTestServer(t, nil)
	got := decodeNatal(t, s, `{
		"datetime": {"date": "1990-06-15", "time": "17:30:00", "timezone": "Europe/Istanbul"},
		"location": {"latitude": 41.0082, "longitude": 28.9784},
		"settings": {"zodiac": "sidereal", "ayanamsha": "lahiri"}
	}`)

	if got.Meta.Ayanamsha != "lahiri" {
		t.Errorf("ayanamsha = %q, want lahiri", got.Meta.Ayanamsha)
	}
	if got.Meta.AyanamshaAt < 23 || got.Meta.AyanamshaAt > 24.5 {
		t.Errorf("ayanamsha_degrees = %.4f, want roughly 23.7", got.Meta.AyanamshaAt)
	}
}

func TestNatalRejectsBadSettings(t *testing.T) {
	s := newTestServer(t, nil)

	cases := []struct{ name, body, field string }{
		{
			"unknown body",
			`{"datetime":{"date":"1990-06-15","time":"17:30","timezone":"UTC"},
			  "location":{"latitude":41,"longitude":29},
			  "settings":{"bodies":["sun","nibiru"]}}`,
			"settings.bodies",
		},
		{
			"unknown house system",
			`{"datetime":{"date":"1990-06-15","time":"17:30","timezone":"UTC"},
			  "location":{"latitude":41,"longitude":29},
			  "settings":{"house_system":"ptolemaic"}}`,
			"settings.house_system",
		},
		{
			"Placidus inside the Arctic Circle",
			`{"datetime":{"date":"1990-06-15","time":"17:30","timezone":"UTC"},
			  "location":{"latitude":78.2,"longitude":15.6}}`,
			"settings.house_system",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := postJSON(t, s, "/v1/natal", tc.body)
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

// TestNatalAspectsSerialiseAsArray checks that a chart with no aspects returns
// an empty array rather than null, so a client can iterate without a guard.
func TestNatalAspectsSerialiseAsArray(t *testing.T) {
	rec := postJSON(t, newTestServer(t, nil), "/v1/natal", `{
		"datetime": {"date": "1990-06-15", "time": "17:30:00", "timezone": "Europe/Istanbul"},
		"location": {"latitude": 41.0082, "longitude": 28.9784},
		"settings": {"bodies": ["sun"], "aspects": {"types": ["quintile"]}}
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("could not decode body: %v", err)
	}
	if string(raw["aspects"]) != "[]" {
		t.Errorf("aspects = %s, want []", raw["aspects"])
	}
}
