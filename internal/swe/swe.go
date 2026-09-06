// Package swe wraps Swiss Ephemeris so it can be used safely from concurrent
// code. For the raw cgo calls see internal/libswe.
package swe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/casbek/api-swiss-ephemeris/internal/libswe"
)

// Config holds the settings a Calculator is built from.
type Config struct {
	// EphePath is the directory holding the .se1 ephemeris files.
	EphePath string

	// Workers is the number of OS threads to reserve. Zero means one. It
	// cannot exceed one on Windows; see newPool.
	Workers int
}

// Calculator provides concurrency-safe access to Swiss Ephemeris. All of its
// methods may be called from multiple goroutines.
type Calculator struct {
	p       *pool
	version string
}

// New validates the ephemeris directory and starts the calculator.
func New(cfg Config) (*Calculator, error) {
	if cfg.EphePath == "" {
		return nil, fmt.Errorf("swe: EphePath must not be empty")
	}
	abs, err := filepath.Abs(cfg.EphePath)
	if err != nil {
		return nil, fmt.Errorf("swe: could not resolve ephemeris path: %w", err)
	}
	if err := checkEpheDir(abs); err != nil {
		return nil, err
	}

	p, err := newPool(abs, cfg.Workers)
	if err != nil {
		return nil, err
	}

	c := &Calculator{p: p}
	// Read the version once so later calls need not enter a worker thread.
	if err := p.do(context.Background(), func() {
		c.version = libswe.Version()
	}); err != nil {
		p.close()
		return nil, err
	}
	return c, nil
}

// checkEpheDir verifies that the directory exists and holds at least one .se1
// file.
//
// When the data files are missing Swiss Ephemeris silently falls back to the
// Moshier ephemeris and every result loses precision without raising an error.
// Catching that at startup is far better than discovering it in production.
func checkEpheDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("swe: could not open ephemeris directory %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("swe: ephemeris path is not a directory: %s", dir)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*.se1"))
	if err != nil {
		return fmt.Errorf("swe: could not scan ephemeris directory: %w", err)
	}
	if len(matches) == 0 {
		return fmt.Errorf("swe: no .se1 files in %s; sepl_18.se1, semo_18.se1 and seas_18.se1 are required", dir)
	}
	return nil
}

// Version reports the Swiss Ephemeris version in use.
func (c *Calculator) Version() string { return c.version }

// Close stops the worker threads and releases the ephemeris files.
func (c *Calculator) Close() { c.p.close() }

// Do runs fn on a thread owned by Swiss Ephemeris.
//
// Compute a whole chart inside a single Do call, so that every value comes
// from one consistent pass on one thread.
//
// The Session is only valid for the duration of fn and must not escape it.
func (c *Calculator) Do(ctx context.Context, fn func(s *Session) error) error {
	var inner error
	s := &Session{}
	if err := c.p.do(ctx, func() {
		s.valid = true
		defer func() { s.valid = false }()
		inner = fn(s)
	}); err != nil {
		return err
	}
	return inner
}

// Session is a calculation session bound to one Swiss Ephemeris thread. It can
// only be used inside Calculator.Do.
type Session struct {
	valid bool
}

// Options selects how a position is computed.
type Options struct {
	// Sidereal switches to the sidereal zodiac used in Vedic astrology.
	Sidereal bool
	// Ayanamsha selects the offset used when Sidereal is set; see the
	// libswe.Sidm constants. Lahiri is the usual choice.
	Ayanamsha int32

	// Topocentric computes positions as seen from the observer rather than
	// from the centre of the Earth. For the Moon the difference reaches
	// about one degree.
	Topocentric bool
	Latitude    float64
	Longitude   float64
	AltitudeM   float64

	// Heliocentric computes positions as seen from the Sun.
	Heliocentric bool

	// Equatorial returns right ascension and declination instead of
	// ecliptic coordinates.
	Equatorial bool

	// TruePositions returns the true geometric position, without the
	// correction for light travel time.
	TruePositions bool

	// NoNutation leaves out nutation, giving the mean equinox of date.
	NoNutation bool
}

// Flags translates the options into a Swiss Ephemeris iflag value.
func (o Options) Flags() int32 {
	// SEFLG_SPEED is always on: retrograde detection depends on the speed
	// and the extra cost is negligible.
	f := int32(libswe.FlagSwieph | libswe.FlagSpeed)
	if o.Sidereal {
		f |= libswe.FlagSidereal
	}
	if o.Topocentric {
		f |= libswe.FlagTopoctr
	}
	if o.Heliocentric {
		f |= libswe.FlagHelctr
	}
	if o.Equatorial {
		f |= libswe.FlagEquatorial
	}
	if o.TruePositions {
		f |= libswe.FlagTruePos
	}
	if o.NoNutation {
		f |= libswe.FlagNoNut
	}
	return f
}

// apply writes the per-request thread state and returns the iflag to use.
//
// These settings are written before every calculation rather than once, since
// worker threads are reused across requests and one request's ayanamsha or
// observer position would otherwise leak into the next.
func (s *Session) apply(o Options) int32 {
	if !s.valid {
		panic("swe: Session used outside Calculator.Do")
	}

	ayan := o.Ayanamsha
	if !o.Sidereal {
		// Reset even in tropical mode, so a value left on the thread by an
		// earlier request cannot affect calls that read the ayanamsha.
		ayan = libswe.SidmFaganBradley
	}
	libswe.SetSidMode(ayan, 0, 0)

	if o.Topocentric {
		libswe.SetTopo(o.Longitude, o.Latitude, o.AltitudeM)
	}
	return o.Flags()
}

// Calc computes the position of body ipl at tjdUT.
func (s *Session) Calc(tjdUT float64, ipl int, o Options) (libswe.CalcResult, error) {
	iflag := s.apply(o)
	res, err := libswe.CalcUT(tjdUT, ipl, iflag)
	if err != nil {
		return res, fmt.Errorf("swe: could not compute %s: %w", libswe.PlanetName(ipl), err)
	}
	return res, nil
}

// Houses computes the house cusps for a moment and place.
func (s *Session) Houses(tjdUT, lat, lon float64, hsys byte, o Options) (libswe.HousesResult, error) {
	iflag := s.apply(o)
	return libswe.HousesEx(tjdUT, iflag, lat, lon, hsys)
}

// HousePos returns the house a position falls in, as a fraction.
func (s *Session) HousePos(armc, lat, eps float64, hsys byte, lon, plat float64) (float64, error) {
	return libswe.HousePos(armc, lat, eps, hsys, lon, plat)
}

// Ayanamsha returns the ayanamsha for the given moment, in degrees.
func (s *Session) Ayanamsha(tjdUT float64, o Options) (float64, error) {
	iflag := s.apply(o)
	return libswe.GetAyanamsa(tjdUT, iflag)
}

// Pheno returns the phase and brightness of a body.
func (s *Session) Pheno(tjdUT float64, ipl int, o Options) (libswe.Pheno, error) {
	iflag := s.apply(o)
	return libswe.PhenoUT(tjdUT, ipl, iflag)
}

// RiseTrans searches for a rise, set or transit. Pass one of the libswe.Calc
// constants as rsmi.
func (s *Session) RiseTrans(tjdUT float64, ipl int, rsmi int32, lat, lon, altM float64, o Options) (jd float64, found bool, err error) {
	iflag := s.apply(o)
	// Passing zero for the pressure lets the library derive it from the
	// altitude.
	return libswe.RiseTrans(tjdUT, ipl, "", iflag, rsmi, lon, lat, altM, 0, 15)
}

// NodesApsides returns the nodes and apsides of a planetary orbit.
func (s *Session) NodesApsides(tjdUT float64, ipl int, method int32, o Options) (libswe.NodesApsides, error) {
	iflag := s.apply(o)
	return libswe.NodApsUT(tjdUT, ipl, iflag, method)
}

// FixStar computes the position of a fixed star.
func (s *Session) FixStar(name string, tjdUT float64, o Options) (string, libswe.CalcResult, error) {
	iflag := s.apply(o)
	return libswe.FixStarUT(name, tjdUT, iflag)
}

// DeltaT returns delta T for the given Julian Day, in days.
func (s *Session) DeltaT(jd float64) (float64, error) {
	return libswe.DeltaT(jd, libswe.FlagSwieph)
}

// UTCToJD converts a UTC date and time to a Julian Day, leap seconds included.
// It does not touch Swiss Ephemeris global state, so it may be called outside
// a session.
func UTCToJD(year, month, day, hour, minute int, sec float64) (et, ut float64, err error) {
	return libswe.UTCToJD(year, month, day, hour, minute, sec, libswe.GregCal)
}

// JDToUTC converts a UT1 Julian Day to UTC calendar values.
func JDToUTC(jdUT float64) (year, month, day, hour, minute int, sec float64) {
	return libswe.JDUT1ToUTC(jdUT, libswe.GregCal)
}

// HousesFromARMC computes house cusps from a sidereal time and a latitude,
// for charts that have no single moment of their own.
func (s *Session) HousesFromARMC(armc, lat, eps float64, hsys byte) (libswe.HousesResult, error) {
	if !s.valid {
		panic("swe: Session used outside Calculator.Do")
	}
	return libswe.HousesARMC(armc, lat, eps, hsys)
}
