package httpapi

import (
	"encoding/json"
	"math"
	"net/http"
	"testing"
)

func TestPositionsEndpoint(t *testing.T) {
	s := newTestServer(t, nil)

	rec := postJSON(t, s, "/v1/positions", `{
		"datetime": {"date": "1990-06-15", "time": "17:30:00", "timezone": "Europe/Istanbul"}
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}

	var got positionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("could not decode body: %v", err)
	}

	if len(got.Bodies) != 13 {
		t.Errorf("got %d bodies, want the 13 of the default set", len(got.Bodies))
	}
	// The same Sun the calculation tests assert.
	if diff := math.Abs(got.Bodies[0].Longitude - 84.2290447); diff > 1e-5 {
		t.Errorf("the Sun is at %.7f, want 84.2290447", got.Bodies[0].Longitude)
	}
	// No place was given, so none is reported.
	if got.Meta.Location != nil {
		t.Errorf("a location came back although none was sent: %+v", got.Meta.Location)
	}
	if got.Meta.Zone.Name != "Europe/Istanbul" || got.Meta.Zone.Offset != "+03:00" {
		t.Errorf("timezone = %+v", got.Meta.Zone)
	}

	// Houses and dignity are what this endpoint deliberately omits.
	for _, b := range got.Bodies {
		if b.House != 0 {
			t.Errorf("%s was placed in house %d", b.Body, b.House)
		}
		if b.Dignity != nil {
			t.Errorf("%s carries a dignity", b.Body)
		}
	}
}

func TestPositionsMatchTheNatalChart(t *testing.T) {
	s := newTestServer(t, nil)

	body := `{"datetime": {"date": "1990-06-15", "time": "17:30:00", "timezone": "Europe/Istanbul"},
	          "location": {"latitude": 41.0082, "longitude": 28.9784}}`

	var positions positionsResponse
	if err := json.Unmarshal(postJSON(t, s, "/v1/positions", body).Body.Bytes(), &positions); err != nil {
		t.Fatalf("could not decode positions: %v", err)
	}
	var natal natalResponse
	if err := json.Unmarshal(postJSON(t, s, "/v1/natal", body).Body.Bytes(), &natal); err != nil {
		t.Fatalf("could not decode chart: %v", err)
	}

	// Asking the narrower question must not give a different answer.
	if len(positions.Bodies) != len(natal.Bodies) {
		t.Fatalf("positions has %d bodies, the chart has %d", len(positions.Bodies), len(natal.Bodies))
	}
	for i := range natal.Bodies {
		if positions.Bodies[i].Longitude != natal.Bodies[i].Longitude {
			t.Errorf("%s is at %.9f here and %.9f in the chart",
				natal.Bodies[i].Body, positions.Bodies[i].Longitude, natal.Bodies[i].Longitude)
		}
	}
}

func TestPositionsRejectTopocentricWithoutAPlace(t *testing.T) {
	s := newTestServer(t, nil)

	rec := postJSON(t, s, "/v1/positions", `{
		"datetime": {"date": "1990-06-15", "time": "17:30:00", "timezone": "Europe/Istanbul"},
		"settings": {"topocentric": true}
	}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body: %s", rec.Code, rec.Body)
	}

	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("could not decode problem: %v", err)
	}
	if len(p.Errors) == 0 || p.Errors[0].Field != "location" {
		t.Errorf("errors = %+v, want the location named", p.Errors)
	}
}

func TestHousesEndpoint(t *testing.T) {
	s := newTestServer(t, nil)

	rec := postJSON(t, s, "/v1/houses", `{
		"datetime": {"date": "1990-06-15", "time": "17:30:00", "timezone": "Europe/Istanbul"},
		"location": {"latitude": 41.0082, "longitude": 28.9784},
		"settings": {"house_system": "placidus"}
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}

	var got housesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("could not decode body: %v", err)
	}

	if got.Houses == nil || len(got.Houses.Cusps) != 12 {
		t.Fatal("the division is missing")
	}
	if got.Meta.HouseSystem != "placidus" {
		t.Errorf("house_system = %q", got.Meta.HouseSystem)
	}

	// The ascendant the calculation tests assert.
	var asc float64
	for _, a := range got.Houses.Angles {
		if a.Body == "ascendant" {
			asc = a.Longitude
		}
	}
	if diff := math.Abs(asc - 227.1761062); diff > 2e-4 {
		t.Errorf("ascendant = %.7f, want 227.1761062", asc)
	}
}

func TestHousesRejectWhatTheyCannotDivide(t *testing.T) {
	s := newTestServer(t, nil)

	cases := []struct{ name, body, field string }{
		{
			"no place to stand", `{
				"datetime": {"date": "1990-06-15", "time": "17:30:00", "timezone": "Europe/Istanbul"}
			}`,
			"location",
		},
		{
			"no time of day", `{
				"datetime": {"date": "1990-06-15", "timezone": "Europe/Istanbul"},
				"location": {"latitude": 41.0082, "longitude": 28.9784}
			}`,
			"datetime.time",
		},
		{
			"Placidus inside the Arctic Circle", `{
				"datetime": {"date": "1990-06-15", "time": "17:30:00", "timezone": "Europe/Istanbul"},
				"location": {"latitude": 78.2, "longitude": 15.6}
			}`,
			"settings.house_system",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := postJSON(t, s, "/v1/houses", tc.body)
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

func TestHousesMatchTheNatalChart(t *testing.T) {
	s := newTestServer(t, nil)

	body := `{"datetime": {"date": "1990-06-15", "time": "17:30:00", "timezone": "Europe/Istanbul"},
	          "location": {"latitude": 41.0082, "longitude": 28.9784},
	          "settings": {"house_system": "koch"}}`

	var houses housesResponse
	if err := json.Unmarshal(postJSON(t, s, "/v1/houses", body).Body.Bytes(), &houses); err != nil {
		t.Fatalf("could not decode houses: %v", err)
	}
	var natal natalResponse
	if err := json.Unmarshal(postJSON(t, s, "/v1/natal", body).Body.Bytes(), &natal); err != nil {
		t.Fatalf("could not decode chart: %v", err)
	}

	for i := range natal.Houses.Cusps {
		if houses.Houses.Cusps[i].Longitude != natal.Houses.Cusps[i].Longitude {
			t.Errorf("cusp %d is at %.9f here and %.9f in the chart",
				i+1, houses.Houses.Cusps[i].Longitude, natal.Houses.Cusps[i].Longitude)
		}
	}
}
