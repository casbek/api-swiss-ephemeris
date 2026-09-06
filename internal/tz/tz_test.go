package tz

import (
	"errors"
	"testing"
	"time"
)

// wantUTC checks that an input resolves to the expected instant.
func wantUTC(t *testing.T, in Input, want string) Resolved {
	t.Helper()

	got, err := Resolve(in)
	if err != nil {
		t.Fatalf("Resolve(%+v) failed: %v", in, err)
	}
	if utc := got.UTC.Format("2006-01-02T15:04:05Z"); utc != want {
		t.Errorf("Resolve(%+v) = %s, want %s", in, utc, want)
	}
	return got
}

// TestHistoricalTurkey checks that the rules in force on the date are used,
// not the rules in force today.
//
// Turkey observed daylight saving until September 2016 and has been on a fixed
// +03 since. A service that applies today's rules to a 1990 birth is an hour
// out, which moves the ascendant by roughly fifteen degrees.
func TestHistoricalTurkey(t *testing.T) {
	cases := []struct {
		name, date, clock, want string
		wantOffset              int
	}{
		{
			name: "summer 1990, daylight saving in force",
			date: "1990-06-15", clock: "17:30:00",
			want: "1990-06-15T14:30:00Z", wantOffset: 3 * 3600,
		},
		{
			name: "spring 1975, standard time",
			date: "1975-03-10", clock: "12:00:00",
			want: "1975-03-10T10:00:00Z", wantOffset: 2 * 3600,
		},
		{
			name: "summer 1960, before daylight saving returned",
			date: "1960-07-01", clock: "12:00:00",
			want: "1960-07-01T10:00:00Z", wantOffset: 2 * 3600,
		},
		{
			name: "summer 2016, the last year of daylight saving",
			date: "2016-06-01", clock: "12:00:00",
			want: "2016-06-01T09:00:00Z", wantOffset: 3 * 3600,
		},
		{
			name: "winter 2020, permanent +03",
			date: "2020-01-15", clock: "12:00:00",
			want: "2020-01-15T09:00:00Z", wantOffset: 3 * 3600,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := wantUTC(t, Input{
				Date: tc.date, Time: tc.clock, Timezone: "Europe/Istanbul",
			}, tc.want)

			if got.Zone.OffsetSeconds != tc.wantOffset {
				t.Errorf("offset = %d, want %d", got.Zone.OffsetSeconds, tc.wantOffset)
			}
			if got.Anomaly != AnomalyNone {
				t.Errorf("anomaly = %q, want none", got.Anomaly)
			}
			if got.Zone.Source != SourceIANA {
				t.Errorf("source = %q, want iana", got.Zone.Source)
			}
		})
	}
}

// TestNonexistentLocalTime checks readings the clocks jumped over.
//
// Turkey moved from 03:00 to 04:00 on 27 March 2016, so 03:30 never happened
// that day. Go returns a shifted instant without complaint; the point of this
// test is that the shift is reported rather than passed off as a fact.
func TestNonexistentLocalTime(t *testing.T) {
	got, err := Resolve(Input{
		Date: "2016-03-27", Time: "03:30:00", Timezone: "Europe/Istanbul",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got.Anomaly != AnomalyNonexistent {
		t.Errorf("anomaly = %q, want nonexistent", got.Anomaly)
	}
	// The clocks read 04:30 at the instant returned.
	if h, m := got.Local.Hour(), got.Local.Minute(); h != 4 || m != 30 {
		t.Errorf("local = %02d:%02d, want the 04:30 the clock showed after the jump", h, m)
	}

	// The readings on either side of the gap are ordinary.
	for _, clock := range []string{"02:30:00", "05:30:00"} {
		got, err := Resolve(Input{
			Date: "2016-03-27", Time: clock, Timezone: "Europe/Istanbul",
		})
		if err != nil {
			t.Fatalf("Resolve(%s) failed: %v", clock, err)
		}
		if got.Anomaly != AnomalyNone {
			t.Errorf("%s: anomaly = %q, want none", clock, got.Anomaly)
		}
	}
}

// TestAmbiguousLocalTime checks readings that happened twice.
//
// Turkey moved from 04:00 back to 03:00 on 8 November 2015, so every reading
// from 03:00 to 03:59 happened twice, an hour apart. A birth recorded as 03:30
// that morning cannot be placed without more information, and saying so is
// more useful than picking one and sounding certain.
func TestAmbiguousLocalTime(t *testing.T) {
	got, err := Resolve(Input{
		Date: "2015-11-08", Time: "03:30:00", Timezone: "Europe/Istanbul",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got.Anomaly != AnomalyAmbiguous {
		t.Fatalf("anomaly = %q, want ambiguous", got.Anomaly)
	}
	if len(got.Alternatives) != 2 {
		t.Fatalf("got %d candidate instants, want 2", len(got.Alternatives))
	}

	first := got.Alternatives[0].Format("2006-01-02T15:04:05Z")
	second := got.Alternatives[1].Format("2006-01-02T15:04:05Z")
	if first != "2015-11-08T00:30:00Z" || second != "2015-11-08T01:30:00Z" {
		t.Errorf("candidates = %s and %s, want 00:30Z and 01:30Z", first, second)
	}
	if !got.Alternatives[0].Before(got.Alternatives[1]) {
		t.Error("candidates are not in chronological order")
	}

	// An unambiguous reading the same morning must not be flagged.
	plain, err := Resolve(Input{
		Date: "2015-11-08", Time: "02:30:00", Timezone: "Europe/Istanbul",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if plain.Anomaly != AnomalyNone {
		t.Errorf("02:30 anomaly = %q, want none", plain.Anomaly)
	}
}

// TestAnomaliesElsewhere checks the same detection against a different zone,
// so it is not tuned to Turkey's history.
func TestAnomaliesElsewhere(t *testing.T) {
	cases := []struct {
		zone, date, clock string
		want              Anomaly
	}{
		{"Europe/Berlin", "2024-03-31", "02:30:00", AnomalyNonexistent},
		{"Europe/Berlin", "2024-10-27", "02:30:00", AnomalyAmbiguous},
		{"Europe/Berlin", "2024-06-15", "02:30:00", AnomalyNone},
		{"America/New_York", "2024-03-10", "02:30:00", AnomalyNonexistent},
		{"America/New_York", "2024-11-03", "01:30:00", AnomalyAmbiguous},
		// Australia moves in the opposite direction to the northern
		// hemisphere, which catches a detector that assumed a season.
		{"Australia/Sydney", "2024-10-06", "02:30:00", AnomalyNonexistent},
		{"Australia/Sydney", "2024-04-07", "02:30:00", AnomalyAmbiguous},
		// A zone with no daylight saving at all.
		{"Asia/Tokyo", "2024-03-31", "02:30:00", AnomalyNone},
	}

	for _, tc := range cases {
		got, err := Resolve(Input{Date: tc.date, Time: tc.clock, Timezone: tc.zone})
		if err != nil {
			t.Errorf("%s %s %s: %v", tc.zone, tc.date, tc.clock, err)
			continue
		}
		if got.Anomaly != tc.want {
			t.Errorf("%s %s %s: anomaly = %q, want %q",
				tc.zone, tc.date, tc.clock, got.Anomaly, tc.want)
		}
	}
}

// TestFixedOffsetIsTakenLiterally checks that an explicit offset bypasses the
// daylight saving rules entirely, which is the point of offering it.
func TestFixedOffsetIsTakenLiterally(t *testing.T) {
	got := wantUTC(t, Input{
		Date: "1990-06-15", Time: "17:30:00", UTCOffset: "+02:00",
	}, "1990-06-15T15:30:00Z")

	if got.Zone.Source != SourceFixedOffset {
		t.Errorf("source = %q, want fixed_offset", got.Zone.Source)
	}
	if got.Zone.OffsetSeconds != 2*3600 {
		t.Errorf("offset = %d, want 7200", got.Zone.OffsetSeconds)
	}
	// The named zone would have given +03 on this date. The whole point of a
	// fixed offset is that it is not second guessed.
	named := wantUTC(t, Input{
		Date: "1990-06-15", Time: "17:30:00", Timezone: "Europe/Istanbul",
	}, "1990-06-15T14:30:00Z")
	if named.UTC.Equal(got.UTC) {
		t.Error("the fixed offset was overridden by the zone rules")
	}
}

func TestOffsetInTimeString(t *testing.T) {
	cases := []struct {
		clock, want string
	}{
		{"14:30:00Z", "1990-06-15T14:30:00Z"},
		{"14:30Z", "1990-06-15T14:30:00Z"},
		{"17:30:00+03:00", "1990-06-15T14:30:00Z"},
		{"17:30+03:00", "1990-06-15T14:30:00Z"},
		{"17:30:00+0300", "1990-06-15T14:30:00Z"},
		{"09:30:00-05:00", "1990-06-15T14:30:00Z"},
	}
	for _, tc := range cases {
		wantUTC(t, Input{Date: "1990-06-15", Time: tc.clock}, tc.want)
	}
}

func TestUnknownTimeUsesMidday(t *testing.T) {
	got, err := Resolve(Input{Date: "1990-06-15", Timezone: "Europe/Istanbul"})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got.TimeKnown {
		t.Error("TimeKnown is true although no time was given")
	}
	if got.Local.Hour() != 12 || got.Local.Minute() != 0 {
		t.Errorf("local = %02d:%02d, want midday", got.Local.Hour(), got.Local.Minute())
	}

	// A time that was given must be marked as known.
	got, err = Resolve(Input{Date: "1990-06-15", Time: "17:30", Timezone: "Europe/Istanbul"})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if !got.TimeKnown {
		t.Error("TimeKnown is false although a time was given")
	}
}

func TestContradictoryInputsAreRejected(t *testing.T) {
	cases := []struct {
		name  string
		in    Input
		field string
	}{
		{
			name:  "zone and offset together",
			in:    Input{Date: "1990-06-15", Time: "17:30", Timezone: "Europe/Istanbul", UTCOffset: "+03:00"},
			field: "datetime.timezone",
		},
		{
			name:  "offset in the time and a zone",
			in:    Input{Date: "1990-06-15", Time: "17:30+03:00", Timezone: "Europe/Istanbul"},
			field: "datetime.time",
		},
		{
			name:  "offset in the time and an offset field",
			in:    Input{Date: "1990-06-15", Time: "17:30Z", UTCOffset: "+03:00"},
			field: "datetime.time",
		},
		{
			name:  "no zone at all",
			in:    Input{Date: "1990-06-15", Time: "17:30"},
			field: "datetime.timezone",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Resolve(tc.in)
			if err == nil {
				t.Fatal("expected an error")
			}
			var fe *FieldError
			if !errors.As(err, &fe) {
				t.Fatalf("error is %T, want a *FieldError so the HTTP layer can point at the field", err)
			}
			if fe.Field != tc.field {
				t.Errorf("field = %q, want %q", fe.Field, tc.field)
			}
		})
	}
}

func TestInvalidInputsAreRejected(t *testing.T) {
	cases := []struct {
		name  string
		in    Input
		field string
	}{
		{"missing date", Input{Time: "17:30", Timezone: "UTC"}, "datetime.date"},
		{"day-first date", Input{Date: "15-06-1990", Time: "17:30", Timezone: "UTC"}, "datetime.date"},
		{"impossible day", Input{Date: "1990-02-30", Time: "17:30", Timezone: "UTC"}, "datetime.date"},
		{"impossible month", Input{Date: "1990-13-01", Time: "17:30", Timezone: "UTC"}, "datetime.date"},
		{"bad time", Input{Date: "1990-06-15", Time: "25:30", Timezone: "UTC"}, "datetime.time"},
		{"bad time text", Input{Date: "1990-06-15", Time: "half past five", Timezone: "UTC"}, "datetime.time"},
		{"unknown zone", Input{Date: "1990-06-15", Time: "17:30", Timezone: "Europe/Constantinople"}, "datetime.timezone"},
		{"server local zone", Input{Date: "1990-06-15", Time: "17:30", Timezone: "Local"}, "datetime.timezone"},
		{"offset without sign", Input{Date: "1990-06-15", Time: "17:30", UTCOffset: "03:00"}, "datetime.utc_offset"},
		{"offset out of range", Input{Date: "1990-06-15", Time: "17:30", UTCOffset: "+19:00"}, "datetime.utc_offset"},
		{"offset minutes out of range", Input{Date: "1990-06-15", Time: "17:30", UTCOffset: "+03:75"}, "datetime.utc_offset"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Resolve(tc.in)
			if err == nil {
				t.Fatal("expected an error")
			}
			var fe *FieldError
			if !errors.As(err, &fe) {
				t.Fatalf("error is %T, want a *FieldError", err)
			}
			if fe.Field != tc.field {
				t.Errorf("field = %q, want %q (%v)", fe.Field, tc.field, err)
			}
		})
	}
}

// TestZoneDatabaseIsEmbedded checks that zone lookups do not depend on the
// host having a zoneinfo directory, which a minimal container image lacks.
func TestZoneDatabaseIsEmbedded(t *testing.T) {
	t.Setenv("ZONEINFO", "/nonexistent/zoneinfo.zip")

	for _, zone := range []string{"Europe/Istanbul", "America/New_York", "Asia/Tokyo", "UTC"} {
		if _, err := time.LoadLocation(zone); err != nil {
			t.Errorf("%s could not be loaded with no system database: %v", zone, err)
		}
	}
}

func TestOffsetFormatting(t *testing.T) {
	cases := []struct {
		seconds int
		want    string
	}{
		{0, "+00:00"},
		{3 * 3600, "+03:00"},
		{-5 * 3600, "-05:00"},
		{5*3600 + 30*60, "+05:30"},
		{-3*3600 - 30*60, "-03:30"},
		{45 * 60, "+00:45"},
	}
	for _, tc := range cases {
		if got := formatOffset(tc.seconds); got != tc.want {
			t.Errorf("formatOffset(%d) = %q, want %q", tc.seconds, got, tc.want)
		}
	}
}

// TestHalfHourZones checks a zone whose offset is not a whole number of hours,
// since India and Iran are common birth places and a truncating conversion
// would be half an hour out.
func TestHalfHourZones(t *testing.T) {
	wantUTC(t, Input{
		Date: "1990-06-15", Time: "20:00:00", Timezone: "Asia/Kolkata",
	}, "1990-06-15T14:30:00Z")

	// Nepal runs at +05:45, the only quarter hour offset in use.
	got := wantUTC(t, Input{
		Date: "1990-06-15", Time: "20:15:00", Timezone: "Asia/Kathmandu",
	}, "1990-06-15T14:30:00Z")
	if got.Zone.Offset != "+05:45" {
		t.Errorf("offset = %q, want +05:45", got.Zone.Offset)
	}
}
