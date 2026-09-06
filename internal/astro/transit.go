package astro

import (
	"context"

	"github.com/casbek/api-swiss-ephemeris/internal/tz"
)

// TransitRequest asks where the sky is at one moment relative to a birth
// chart.
type TransitRequest struct {
	Natal ChartRequest `json:"natal"`

	// Transit is the moment to look at. Its location defaults to the natal
	// one, which is what a client almost always wants: the transiting
	// houses are read against the place the person lives or was born.
	Transit MomentRequest `json:"transit"`

	// Settings apply to the transiting chart and to the comparison. They
	// default to the natal settings, so a client that wants the same bodies
	// and orbs on both sides need only give them once.
	Settings *SettingsInput `json:"settings,omitempty"`
}

// MomentRequest is a moment, with an optional place.
type MomentRequest struct {
	DateTime tz.Input  `json:"datetime"`
	Location *Location `json:"location,omitempty"`
}

// TransitResult is a birth chart, the sky at another moment, and the aspects
// between them.
type TransitResult struct {
	Natal   *Chart
	Transit *Chart

	// Aspects run from a transiting body to a natal point. The natal angles
	// are included as targets, since a transit to the ascendant or midheaven
	// is among the most watched of all.
	Aspects []Aspect
}

// Transits casts a birth chart, casts the sky at another moment and reports
// how the two relate.
func (e *Engine) Transits(ctx context.Context, req TransitRequest) (*TransitResult, error) {
	natal, err := e.Cast(ctx, req.Natal)
	if err != nil {
		return nil, prefixField(err, "natal")
	}

	settings := req.Natal.Settings
	if req.Settings != nil {
		settings = *req.Settings
	}

	location := req.Natal.Location
	if req.Transit.Location != nil {
		location = *req.Transit.Location
	}

	transit, err := e.Cast(ctx, ChartRequest{
		DateTime: req.Transit.DateTime,
		Location: location,
		Settings: settings,
	})
	if err != nil {
		return nil, prefixField(err, "transit")
	}

	return &TransitResult{
		Natal:   natal,
		Transit: transit,
		Aspects: CrossAspects(transit.Positions, natalTargets(natal),
			transit.Settings.AspectTypes, transit.Settings.Orbs),
	}, nil
}

// natalTargets is everything a transit can aspect: the natal bodies and, when
// the birth time is known, the natal angles.
func natalTargets(natal *Chart) []Position {
	if natal.Houses == nil {
		return natal.Positions
	}
	return append(append([]Position(nil), natal.Positions...), natal.Houses.Angles...)
}

// SynastryRequest compares two birth charts.
type SynastryRequest struct {
	ChartA ChartRequest `json:"chart_a"`
	ChartB ChartRequest `json:"chart_b"`

	// Settings apply to the comparison. They default to the settings of the
	// first chart.
	Settings *SettingsInput `json:"settings,omitempty"`
}

// SynastryResult is two charts and the aspects between them.
type SynastryResult struct {
	ChartA *Chart
	ChartB *Chart

	// Aspects run from a point in the first chart to a point in the second.
	// The angles of both charts take part, since contacts to the ascendant
	// and midheaven carry a lot of weight in this technique.
	Aspects []Aspect
}

// Synastry casts two charts and reports the aspects between them.
func (e *Engine) Synastry(ctx context.Context, req SynastryRequest) (*SynastryResult, error) {
	a, err := e.Cast(ctx, req.ChartA)
	if err != nil {
		return nil, prefixField(err, "chart_a")
	}
	b, err := e.Cast(ctx, req.ChartB)
	if err != nil {
		return nil, prefixField(err, "chart_b")
	}

	settings := a.Settings
	if req.Settings != nil {
		resolved, err := ResolveSettings(*req.Settings, req.ChartA.Location)
		if err != nil {
			return nil, err
		}
		settings = resolved
	}

	return &SynastryResult{
		ChartA: a,
		ChartB: b,
		Aspects: CrossAspects(natalTargets(a), natalTargets(b),
			settings.AspectTypes, settings.Orbs),
	}, nil
}

// prefixField points a validation failure at whichever half of the request
// produced it, so a client is told which chart to correct.
func prefixField(err error, prefix string) error {
	fe, ok := err.(*tz.FieldError)
	if !ok {
		return err
	}
	return &tz.FieldError{Field: prefix + "." + fe.Field, Message: fe.Message}
}
