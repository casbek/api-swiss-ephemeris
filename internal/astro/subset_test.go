package astro

import (
	"context"
	"math"
	"testing"

	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// The two partial endpoints exist to return part of a chart. The check that
// matters most is therefore that the part they return is the same part the
// whole chart contains: if these ever drift from Cast, a client would get a
// different answer depending on which question it asked.

func TestPositionsAgreeWithTheWholeChart(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	chart, err := e.Cast(ctx, referenceRequest)
	if err != nil {
		t.Fatalf("Cast failed: %v", err)
	}

	location := referenceRequest.Location
	only, err := e.Positions(ctx, PositionsRequest{
		DateTime: referenceRequest.DateTime,
		Location: &location,
	})
	if err != nil {
		t.Fatalf("Positions failed: %v", err)
	}

	if len(only.Positions) != len(chart.Positions) {
		t.Fatalf("got %d bodies, the chart has %d", len(only.Positions), len(chart.Positions))
	}
	for i, want := range chart.Positions {
		got := only.Positions[i]
		if got.Body != want.Body {
			t.Fatalf("body %d is %s, the chart has %s", i, got.Body, want.Body)
		}
		if got.Longitude != want.Longitude {
			t.Errorf("%s is at %.9f, the chart has %.9f", got.Body, got.Longitude, want.Longitude)
		}
		if got.Speed != want.Speed || got.Declination != want.Declination {
			t.Errorf("%s differs from the chart in speed or declination", got.Body)
		}
	}
	if only.Obliquity != chart.Obliquity {
		t.Errorf("obliquity = %.9f, the chart has %.9f", only.Obliquity, chart.Obliquity)
	}
}

// TestPositionsCarryNoHouseOrDignity checks what the endpoint deliberately
// leaves out. Reporting a house without having divided the sky, or a dignity
// score whose triplicity depends on a sect that was never determined, would be
// worse than reporting nothing.
func TestPositionsCarryNoHouseOrDignity(t *testing.T) {
	e := newTestEngine(t)

	only, err := e.Positions(context.Background(), PositionsRequest{
		DateTime: referenceRequest.DateTime,
	})
	if err != nil {
		t.Fatalf("Positions failed: %v", err)
	}

	for _, p := range only.Positions {
		if p.House != 0 || p.HousePosition != 0 {
			t.Errorf("%s was placed in house %d, but no houses were calculated", p.Body, p.House)
		}
		if p.Dignity != nil {
			t.Errorf("%s carries a dignity, but without a horizon there is no sect to read "+
				"its triplicity by", p.Body)
		}
	}
}

// TestPositionsNeedNoPlace checks that the endpoint answers without one. Where
// the bodies are is the same question wherever it is asked from.
func TestPositionsNeedNoPlace(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	withPlace, err := e.Positions(ctx, PositionsRequest{
		DateTime: referenceRequest.DateTime,
		Location: &referenceRequest.Location,
	})
	if err != nil {
		t.Fatalf("Positions failed: %v", err)
	}
	withoutPlace, err := e.Positions(ctx, PositionsRequest{
		DateTime: referenceRequest.DateTime,
	})
	if err != nil {
		t.Fatalf("Positions without a location failed: %v", err)
	}

	for i := range withPlace.Positions {
		if withPlace.Positions[i].Longitude != withoutPlace.Positions[i].Longitude {
			t.Errorf("%s moved when a location was given, but a geocentric position "+
				"does not depend on one", withPlace.Positions[i].Body)
		}
	}
}

// TestTopocentricNeedsAPlace checks the one case where the location matters.
// Defaulting it would measure from the point where the equator meets the prime
// meridian and be quietly wrong rather than obviously so.
func TestTopocentricNeedsAPlace(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	_, err := e.Positions(ctx, PositionsRequest{
		DateTime: referenceRequest.DateTime,
		Settings: SettingsInput{Topocentric: true, Bodies: []string{"moon"}},
	})
	if err == nil {
		t.Fatal("topocentric positions without a location should be refused")
	}
	if fe, ok := err.(*tz.FieldError); !ok || fe.Field != "location" {
		t.Errorf("error = %v, want one naming the location", err)
	}

	// With a place it works, and it really is a different answer: the Moon is
	// near enough that where you stand moves it by about a degree.
	location := referenceRequest.Location
	topo, err := e.Positions(ctx, PositionsRequest{
		DateTime: referenceRequest.DateTime,
		Location: &location,
		Settings: SettingsInput{Topocentric: true, Bodies: []string{"moon"}},
	})
	if err != nil {
		t.Fatalf("Positions failed: %v", err)
	}
	geo, err := e.Positions(ctx, PositionsRequest{
		DateTime: referenceRequest.DateTime,
		Settings: SettingsInput{Bodies: []string{"moon"}},
	})
	if err != nil {
		t.Fatalf("Positions failed: %v", err)
	}

	moved := Separation(topo.Positions[0].Longitude, geo.Positions[0].Longitude)
	if moved < 0.1 || moved > 1.5 {
		t.Errorf("the topocentric Moon moved %.4f degrees, want roughly half a degree; "+
			"if it did not move at all the setting was not applied", moved)
	}
}

func TestHousesAgreeWithTheWholeChart(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	chart, err := e.Cast(ctx, ChartRequest{
		DateTime: referenceRequest.DateTime,
		Location: referenceRequest.Location,
		Settings: SettingsInput{HouseSystem: "placidus"},
	})
	if err != nil {
		t.Fatalf("Cast failed: %v", err)
	}

	location := referenceRequest.Location
	only, err := e.Houses(ctx, HousesRequest{
		DateTime: referenceRequest.DateTime,
		Location: &location,
		Settings: SettingsInput{HouseSystem: "placidus"},
	})
	if err != nil {
		t.Fatalf("Houses failed: %v", err)
	}

	if len(only.Houses.Cusps) != len(chart.Houses.Cusps) {
		t.Fatalf("got %d cusps, the chart has %d", len(only.Houses.Cusps), len(chart.Houses.Cusps))
	}
	for i := range chart.Houses.Cusps {
		if only.Houses.Cusps[i].Longitude != chart.Houses.Cusps[i].Longitude {
			t.Errorf("cusp %d is at %.9f, the chart has %.9f",
				i+1, only.Houses.Cusps[i].Longitude, chart.Houses.Cusps[i].Longitude)
		}
	}
	for i := range chart.Houses.Angles {
		if only.Houses.Angles[i].Longitude != chart.Houses.Angles[i].Longitude {
			t.Errorf("the %s is at %.9f, the chart has %.9f",
				chart.Houses.Angles[i].Body,
				only.Houses.Angles[i].Longitude, chart.Houses.Angles[i].Longitude)
		}
	}
	if only.Houses.ARMC != chart.Houses.ARMC {
		t.Errorf("ARMC = %.9f, the chart has %.9f", only.Houses.ARMC, chart.Houses.ARMC)
	}

	// The reference values, so this is anchored to something outside the code
	// as well as to the chart.
	if diff := math.Abs(only.Houses.Angles[0].Longitude - 227.1761062); diff > 2e-4 {
		t.Errorf("ascendant = %.7f, want 227.1761062", only.Houses.Angles[0].Longitude)
	}
}

func TestHousesRefuseWhatTheyCannotDivide(t *testing.T) {
	e := newTestEngine(t)
	location := referenceRequest.Location

	cases := []struct {
		name  string
		req   HousesRequest
		field string
	}{
		{
			"no place to stand",
			HousesRequest{DateTime: referenceRequest.DateTime},
			"location",
		},
		{
			"no time of day",
			HousesRequest{
				DateTime: tz.Input{Date: "1990-06-15", Timezone: "Europe/Istanbul"},
				Location: &location,
			},
			"datetime.time",
		},
		{
			"Placidus inside the Arctic Circle",
			HousesRequest{
				DateTime: referenceRequest.DateTime,
				Location: &Location{Latitude: 78.2, Longitude: 15.6},
			},
			"settings.house_system",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.Houses(context.Background(), tc.req)
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

// TestHousesCarryNoBodies checks that the endpoint returns the division and
// nothing placed in it.
func TestHousesCarryNoBodies(t *testing.T) {
	e := newTestEngine(t)
	location := referenceRequest.Location

	only, err := e.Houses(context.Background(), HousesRequest{
		DateTime: referenceRequest.DateTime,
		Location: &location,
	})
	if err != nil {
		t.Fatalf("Houses failed: %v", err)
	}

	if only.Houses == nil || len(only.Houses.Cusps) != 12 {
		t.Fatal("the division is missing")
	}
	// The angles are part of the division, not bodies placed in it.
	if len(only.Houses.Angles) != 5 {
		t.Errorf("got %d angles, want the five the division defines", len(only.Houses.Angles))
	}
}

// TestSubsetsCarryNakshatrasWhenSidereal checks that the two endpoints answer
// the mansion question the same way every other endpoint does.
func TestSubsetsCarryNakshatrasWhenSidereal(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()
	location := referenceRequest.Location

	tropical, err := e.Positions(ctx, PositionsRequest{DateTime: referenceRequest.DateTime})
	if err != nil {
		t.Fatalf("Positions failed: %v", err)
	}
	for _, p := range tropical.Positions {
		if p.Nakshatra != nil {
			t.Errorf("%s carries a nakshatra in a tropical request", p.Body)
		}
	}

	sidereal, err := e.Positions(ctx, PositionsRequest{
		DateTime: referenceRequest.DateTime,
		Settings: SettingsInput{Zodiac: "sidereal"},
	})
	if err != nil {
		t.Fatalf("Positions failed: %v", err)
	}
	for _, p := range sidereal.Positions {
		if p.Nakshatra == nil {
			t.Errorf("%s carries no nakshatra in a sidereal request", p.Body)
		}
	}
	if sidereal.Ayanamsha < 23 || sidereal.Ayanamsha > 24.5 {
		t.Errorf("ayanamsha = %.4f, want roughly 23.7 for 1990", sidereal.Ayanamsha)
	}

	houses, err := e.Houses(ctx, HousesRequest{
		DateTime: referenceRequest.DateTime,
		Location: &location,
		Settings: SettingsInput{Zodiac: "sidereal", HouseSystem: "whole_sign"},
	})
	if err != nil {
		t.Fatalf("Houses failed: %v", err)
	}
	for _, a := range houses.Houses.Angles {
		if a.Nakshatra == nil {
			t.Errorf("the %s carries no nakshatra in a sidereal request", a.Body)
		}
	}
}

// TestEphemerisCarriesNakshatras checks the endpoint that was missing them
// before the mansion logic was brought into one place.
func TestEphemerisCarriesNakshatras(t *testing.T) {
	e := newTestEngine(t)

	result, err := e.Ephemeris(context.Background(), EphemerisRequest{
		RangeRequest: span("2024-01-01", "2024-01-03"),
		Settings:     SettingsInput{Zodiac: "sidereal", Bodies: []string{"sun", "moon"}},
	})
	if err != nil {
		t.Fatalf("Ephemeris failed: %v", err)
	}

	for _, row := range result.Rows {
		for _, p := range row.Positions {
			if p.Nakshatra == nil {
				t.Fatalf("%s carries no nakshatra in a sidereal ephemeris", p.Body)
			}
		}
	}
}
