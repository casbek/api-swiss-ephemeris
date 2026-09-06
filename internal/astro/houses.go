package astro

// Cusp is the start of one house.
type Cusp struct {
	House        int     `json:"house"`
	Longitude    float64 `json:"longitude"`
	Sign         string  `json:"sign"`
	SignIndex    int     `json:"sign_index"`
	DegreeInSign float64 `json:"degree_in_sign"`
	Formatted    string  `json:"formatted"`
}

// Houses is the house division of a chart, with its angles.
type Houses struct {
	System      string `json:"system"`
	SystemLabel string `json:"system_label"`

	// Cusps holds twelve houses in order, or thirty six sectors for the
	// Gauquelin division.
	Cusps []Cusp `json:"cusps"`

	// Angles holds the ascendant, midheaven, descendant, imum coeli and
	// vertex, in that order. They are ordinary positions so they can carry
	// aspects like any other point.
	Angles []Position `json:"angles"`

	// ARMC is the right ascension of the midheaven, the sidereal time the
	// division was built from. A composite chart is derived from it, since
	// it has no moment of its own to cast for.
	ARMC float64 `json:"armc"`
}

// gauquelinSectors is the number of divisions the Gauquelin system uses
// instead of twelve houses.
const gauquelinSectors = 36

// buildHouses turns a raw house calculation into the response shape.
func buildHouses(system HouseSystem, cusps [37]float64, ascmc [10]float64) *Houses {
	count := 12
	if system.Letter == 'G' {
		count = gauquelinSectors
	}

	h := &Houses{
		System:      system.Name,
		SystemLabel: system.Label,
		Cusps:       make([]Cusp, 0, count),
		ARMC:        Normalize(ascmc[2]),
	}

	for i := 1; i <= count; i++ {
		lon := Normalize(cusps[i])
		sign := SignAt(lon)
		h.Cusps = append(h.Cusps, Cusp{
			House:        i,
			Longitude:    lon,
			Sign:         sign.Name,
			SignIndex:    sign.Index,
			DegreeInSign: DegreeInSign(lon),
			Formatted:    FormatPosition(lon),
		})
	}

	asc, mc, vertex := ascmc[0], ascmc[1], ascmc[3]
	h.Angles = []Position{
		newPosition("ascendant", "Ascendant", asc),
		newPosition("midheaven", "Midheaven", mc),
		// The descendant and imum coeli are always opposite the other two,
		// so they are derived rather than asked for again.
		newPosition("descendant", "Descendant", asc+180),
		newPosition("imum_coeli", "Imum Coeli", mc+180),
		newPosition("vertex", "Vertex", vertex),
	}
	return h
}

// houseOf reports which house a longitude falls in and how far through it,
// so 8.53 means 53 percent of the way through the eighth house.
//
// The house is found by asking which pair of cusps the longitude lies between,
// which is what practitioners mean by the house a body is in and gives the
// same answer in both zodiacs. It follows that a body's ecliptic latitude does
// not affect the result; for a body well off the ecliptic a more refined
// method can disagree near a cusp.
func (h *Houses) houseOf(longitude float64) (int, float64) {
	if h == nil || len(h.Cusps) == 0 {
		return 0, 0
	}
	lon := Normalize(longitude)
	n := len(h.Cusps)

	for i := 0; i < n; i++ {
		start := h.Cusps[i].Longitude
		end := h.Cusps[(i+1)%n].Longitude

		span := Normalize(end - start)
		if span == 0 {
			// Two cusps at the same degree would make an empty house; there
			// is nothing to place in it.
			continue
		}
		if offset := Normalize(lon - start); offset < span {
			return h.Cusps[i].House, float64(h.Cusps[i].House) + offset/span
		}
	}
	// Every point on the circle lies in some house, so this is unreachable
	// unless the cusps are degenerate.
	return 0, 0
}

// assign fills in the house fields of a position.
func (h *Houses) assign(p *Position) {
	house, position := h.houseOf(p.Longitude)
	p.House = house
	p.HousePosition = position
}
