package astro

import (
	"fmt"
	"math"

	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// HighLatitude is where the house systems that divide the ecliptic by its
// intersection with the horizon start to break down. Beyond it Placidus and
// Koch can produce cusps that are out of order or undefined, so a request for
// one of them is refused rather than answered with nonsense.
const HighLatitude = 66.0

// altitude bounds, generous enough for the Dead Sea shore and for Everest.
const (
	minAltitudeM = -500
	maxAltitudeM = 9000
)

// Location is a place on Earth. Latitude is positive north and longitude
// positive east.
type Location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	AltitudeM float64 `json:"altitude_m,omitempty"`
}

// Validate checks that the coordinates describe a real place.
func (l Location) Validate() error {
	switch {
	case math.IsNaN(l.Latitude) || math.IsInf(l.Latitude, 0):
		return &tz.FieldError{Field: "location.latitude", Message: "must be a number"}
	case l.Latitude < -90 || l.Latitude > 90:
		return &tz.FieldError{
			Field:   "location.latitude",
			Message: fmt.Sprintf("%g is outside the range -90 to 90", l.Latitude),
		}
	case math.IsNaN(l.Longitude) || math.IsInf(l.Longitude, 0):
		return &tz.FieldError{Field: "location.longitude", Message: "must be a number"}
	case l.Longitude < -180 || l.Longitude > 180:
		return &tz.FieldError{
			Field:   "location.longitude",
			Message: fmt.Sprintf("%g is outside the range -180 to 180", l.Longitude),
		}
	case math.IsNaN(l.AltitudeM) || math.IsInf(l.AltitudeM, 0):
		return &tz.FieldError{Field: "location.altitude_m", Message: "must be a number"}
	case l.AltitudeM < minAltitudeM || l.AltitudeM > maxAltitudeM:
		return &tz.FieldError{
			Field: "location.altitude_m",
			Message: fmt.Sprintf("%g is outside the range %d to %d metres",
				l.AltitudeM, minAltitudeM, maxAltitudeM),
		}
	}
	return nil
}

// IsHighLatitude reports whether the place is far enough from the equator for
// the quadrant house systems to be unreliable.
func (l Location) IsHighLatitude() bool {
	return math.Abs(l.Latitude) >= HighLatitude
}

// CheckHouseSystem reports whether a house system can be used at this place.
func (l Location) CheckHouseSystem(h HouseSystem) error {
	if h.FailsAtHighLatitude && l.IsHighLatitude() {
		return &tz.FieldError{
			Field: "settings.house_system",
			Message: fmt.Sprintf(
				"%s is undefined at latitude %.4f, beyond %.0f degrees; use whole_sign, equal or porphyry instead",
				h.Name, l.Latitude, HighLatitude),
		}
	}
	return nil
}
