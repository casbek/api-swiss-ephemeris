package astro

// Position is where a body is, in every form a client is likely to need.
//
// Both the raw longitude and its decomposition into sign and degree are
// present. A client that wants to compute never has to parse a string, and one
// that wants to display never has to divide the circle.
type Position struct {
	Body  string `json:"body"`
	Label string `json:"label,omitempty"`

	Longitude    float64 `json:"longitude"`
	Sign         string  `json:"sign"`
	SignIndex    int     `json:"sign_index"`
	DegreeInSign float64 `json:"degree_in_sign"`
	Formatted    string  `json:"formatted"`
	Latitude     float64 `json:"latitude"`
	DistanceAU   float64 `json:"distance_au,omitempty"`
	Declination  float64 `json:"declination,omitempty"`

	// Speed is the change in longitude per day. Negative means retrograde.
	Speed        float64 `json:"speed"`
	IsRetrograde bool    `json:"is_retrograde"`

	// House is the house the body falls in, and HousePosition the fractional
	// position within it, so 8.53 is 53 percent of the way through the
	// eighth house. Both are omitted when houses were not calculated.
	House         int     `json:"house,omitempty"`
	HousePosition float64 `json:"house_position,omitempty"`

	// Dignity is present only for the seven classical planets.
	Dignity *Dignity `json:"dignity,omitempty"`

	// Nakshatra is the lunar mansion the position falls in, with its quarter.
	// It is present only in a sidereal chart: the mansions are fixed to the
	// stars rather than to the equinox, so reading them off a tropical
	// longitude would put every one of them about twenty four degrees out.
	Nakshatra *NakshatraPlacement `json:"nakshatra,omitempty"`
}

// newPosition fills in everything that follows from a longitude.
func newPosition(name, label string, longitude float64) Position {
	sign := SignAt(longitude)
	lon := Normalize(longitude)

	return Position{
		Body:         name,
		Label:        label,
		Longitude:    lon,
		Sign:         sign.Name,
		SignIndex:    sign.Index,
		DegreeInSign: DegreeInSign(lon),
		Formatted:    FormatPosition(lon),
	}
}

// orb returns the base orb this position is allowed, falling back to the
// catalogue when the settings carry no entry for it.
func (p Position) orb(orbs map[string]float64) float64 {
	if v, ok := orbs[p.Body]; ok {
		return v
	}
	if b, ok := LookupBody(p.Body); ok {
		return b.BaseOrb
	}
	return 0
}
