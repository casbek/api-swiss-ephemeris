package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// postJSON sends a request body to the server under test.
func postJSON(t *testing.T, s *Server, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func decodeTime(t *testing.T, rec *httptest.ResponseRecorder) timeResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	var out timeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("could not decode body: %v", err)
	}
	return out
}

// TestTimeResolvesHistoricalZone checks the whole path from a local reading to
// a Julian Day, against the moment the calculation tests use as their
// reference.
func TestTimeResolvesHistoricalZone(t *testing.T) {
	s := newTestServer(t, nil)

	rec := postJSON(t, s, "/v1/time", `{
		"datetime": {"date": "1990-06-15", "time": "17:30:00", "timezone": "Europe/Istanbul"}
	}`)
	got := decodeTime(t, rec)

	if !strings.HasPrefix(got.UTC, "1990-06-15T14:30:00") {
		t.Errorf("utc = %q, want 1990-06-15T14:30:00Z", got.UTC)
	}
	if got.Zone.Offset != "+03:00" {
		t.Errorf("offset = %q, want +03:00; Turkey observed daylight saving in 1990", got.Zone.Offset)
	}
	if !got.Zone.IsDST {
		t.Error("is_dst is false, but daylight saving was in force")
	}

	// This is the Julian Day the reference values in the swe package use.
	if diff := got.Julia.UT - 2448058.104166667; diff > 1e-4 || diff < -1e-4 {
		t.Errorf("julian_day.ut = %.9f, want 2448058.104166667", got.Julia.UT)
	}
	// Terrestrial Time runs ahead of Universal Time, never behind.
	if got.Julia.TT <= got.Julia.UT {
		t.Errorf("julian_day.tt = %.9f is not ahead of ut %.9f", got.Julia.TT, got.Julia.UT)
	}
	if got.Meta.DeltaTSeconds < 50 || got.Meta.DeltaTSeconds > 65 {
		t.Errorf("delta_t = %.3f s, want roughly 57 s for 1990", got.Meta.DeltaTSeconds)
	}
	if got.Flags.Anomaly != "" {
		t.Errorf("anomaly = %q, want none", got.Flags.Anomaly)
	}
	if !got.Flags.TimeKnown {
		t.Error("time_known is false although a time was given")
	}
}

func TestTimeReportsAmbiguousReading(t *testing.T) {
	s := newTestServer(t, nil)

	rec := postJSON(t, s, "/v1/time", `{
		"datetime": {"date": "2015-11-08", "time": "03:30:00", "timezone": "Europe/Istanbul"}
	}`)
	got := decodeTime(t, rec)

	if got.Flags.Anomaly != "ambiguous" {
		t.Fatalf("anomaly = %q, want ambiguous", got.Flags.Anomaly)
	}
	if len(got.Flags.Candidates) != 2 {
		t.Fatalf("got %d candidates, want both instants", len(got.Flags.Candidates))
	}
	// A client has to be told, or it will trust a chart that could be an
	// hour out.
	if len(got.Notes) == 0 {
		t.Error("no note explains the ambiguity")
	}
}

func TestTimeReportsNonexistentReading(t *testing.T) {
	s := newTestServer(t, nil)

	rec := postJSON(t, s, "/v1/time", `{
		"datetime": {"date": "2016-03-27", "time": "03:30:00", "timezone": "Europe/Istanbul"}
	}`)
	got := decodeTime(t, rec)

	if got.Flags.Anomaly != "nonexistent" {
		t.Errorf("anomaly = %q, want nonexistent", got.Flags.Anomaly)
	}
	if len(got.Notes) == 0 {
		t.Error("no note explains that the reading never occurred")
	}
}

func TestTimeWithoutTimeIsFlagged(t *testing.T) {
	s := newTestServer(t, nil)

	rec := postJSON(t, s, "/v1/time", `{
		"datetime": {"date": "1990-06-15", "timezone": "Europe/Istanbul"}
	}`)
	got := decodeTime(t, rec)

	if got.Flags.TimeKnown {
		t.Error("time_known is true although no time was sent")
	}
	if !strings.Contains(strings.Join(got.Notes, " "), "midday") {
		t.Errorf("notes do not mention the assumption: %v", got.Notes)
	}
}

func TestTimeAcceptsAllThreeForms(t *testing.T) {
	s := newTestServer(t, nil)

	cases := []struct {
		name, body string
	}{
		{"iana zone", `{"datetime":{"date":"1990-06-15","time":"17:30:00","timezone":"Europe/Istanbul"}}`},
		{"fixed offset", `{"datetime":{"date":"1990-06-15","time":"17:30:00","utc_offset":"+03:00"}}`},
		{"offset in time", `{"datetime":{"date":"1990-06-15","time":"17:30:00+03:00"}}`},
		{"utc in time", `{"datetime":{"date":"1990-06-15","time":"14:30:00Z"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decodeTime(t, postJSON(t, s, "/v1/time", tc.body))
			if !strings.HasPrefix(got.UTC, "1990-06-15T14:30:00") {
				t.Errorf("utc = %q, want 1990-06-15T14:30:00Z", got.UTC)
			}
		})
	}
}

func TestTimeRejectsBadInput(t *testing.T) {
	s := newTestServer(t, nil)

	cases := []struct {
		name, body, field string
		status            int
	}{
		{
			name: "unknown zone", status: http.StatusUnprocessableEntity,
			body:  `{"datetime":{"date":"1990-06-15","time":"17:30","timezone":"Europe/Constantinople"}}`,
			field: "datetime.timezone",
		},
		{
			name: "year before the ephemeris", status: http.StatusUnprocessableEntity,
			body:  `{"datetime":{"date":"1750-06-15","time":"17:30","timezone":"Europe/Istanbul"}}`,
			field: "datetime.date",
		},
		{
			name: "year after the ephemeris", status: http.StatusUnprocessableEntity,
			body:  `{"datetime":{"date":"2450-06-15","time":"17:30","timezone":"Europe/Istanbul"}}`,
			field: "datetime.date",
		},
		{
			name: "impossible latitude", status: http.StatusUnprocessableEntity,
			body: `{"datetime":{"date":"1990-06-15","time":"17:30","timezone":"UTC"},
			        "location":{"latitude":95,"longitude":28}}`,
			field: "location.latitude",
		},
		{
			name: "contradictory zone and offset", status: http.StatusUnprocessableEntity,
			body:  `{"datetime":{"date":"1990-06-15","time":"17:30","timezone":"UTC","utc_offset":"+03:00"}}`,
			field: "datetime.timezone",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := postJSON(t, s, "/v1/time", tc.body)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, tc.status, rec.Body)
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

// TestTimeRejectsUnknownFields checks that a misspelled setting is reported
// rather than silently ignored, which would hand back a chart that is not the
// one asked for.
func TestTimeRejectsUnknownFields(t *testing.T) {
	s := newTestServer(t, nil)

	rec := postJSON(t, s, "/v1/time", `{
		"datetime": {"date": "1990-06-15", "time": "17:30", "timezone": "UTC", "timezon": "typo"}
	}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an unknown field; body: %s", rec.Code, rec.Body)
	}
}

func TestTimeRejectsWrongContentType(t *testing.T) {
	s := newTestServer(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/time",
		strings.NewReader(`{"datetime":{"date":"1990-06-15","time":"17:30","timezone":"UTC"}}`))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestTimeRejectsGet(t *testing.T) {
	s := newTestServer(t, nil)
	if rec := do(t, s, http.MethodGet, "/v1/time", nil); rec.Code != http.StatusNotFound {
		// The mux registers only POST for this path, so a GET falls through
		// to the catch-all handler.
		t.Errorf("status = %d, want the request to be refused", rec.Code)
	}
}
