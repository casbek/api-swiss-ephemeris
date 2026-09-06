// Package libswe contains the raw cgo bindings for the Swiss Ephemeris 2.10.03
// C library.
//
// Do not call these functions directly. Swiss Ephemeris keeps global state,
// which is thread-local on Linux with GCC and shared on Windows. Every call
// must go through the worker pool in internal/swe; see internal/swe/pool.go.
package libswe

/*
#cgo CFLAGS: -O2 -std=gnu99 -Wno-unused-but-set-variable -Wno-unused-variable -Wno-implicit-fallthrough -Wno-sign-compare
#cgo linux LDFLAGS: -lm -ldl
#cgo darwin LDFLAGS: -lm
#cgo windows LDFLAGS: -lm

#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include "swephexp.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

// errBufSize must match the AS_MAXCH constant of Swiss Ephemeris.
const errBufSize = 256

// maxStarName is SE_MAX_STNAME + 1, the buffer size swe_fixstar2_ut expects.
const maxStarName = 256

// newErrBuf allocates the error buffer passed to C and returns it along with
// its release function.
func newErrBuf() (*C.char, func()) {
	buf := (*C.char)(C.calloc(errBufSize, 1))
	return buf, func() { C.free(unsafe.Pointer(buf)) }
}

// errFrom converts a C error buffer into a Go error, or nil if it is empty.
func errFrom(buf *C.char) error {
	msg := C.GoString(buf)
	if msg == "" {
		return nil
	}
	return errors.New(msg)
}

// SetEphePath sets the directory holding the ephemeris data files. It must be
// called separately for every OS thread, since the state is thread-local.
func SetEphePath(path string) {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	C.swe_set_ephe_path(cPath)
}

// Close releases the open ephemeris files and their memory.
func Close() { C.swe_close() }

// Version reports the version of the underlying library.
func Version() string {
	buf := (*C.char)(C.calloc(errBufSize, 1))
	defer C.free(unsafe.Pointer(buf))
	C.swe_version(buf)
	return C.GoString(buf)
}

// Julday converts a calendar date to a Julian Day. hour is the decimal hour in
// UT. Pass GregCal or JulCal for gregflag.
func Julday(year, month, day int, hour float64, gregflag int) float64 {
	return float64(C.swe_julday(C.int(year), C.int(month), C.int(day),
		C.double(hour), C.int(gregflag)))
}

// Revjul converts a Julian Day back to a calendar date.
func Revjul(jd float64, gregflag int) (year, month, day int, hour float64) {
	var y, m, d C.int
	var h C.double
	C.swe_revjul(C.double(jd), C.int(gregflag), &y, &m, &d, &h)
	return int(y), int(m), int(d), float64(h)
}

// UTCToJD converts a UTC date and time to a Julian Day, accounting for leap
// seconds. It returns the moment in TT (Terrestrial Time) and in UT1.
func UTCToJD(year, month, day, hour, minute int, sec float64, gregflag int) (et, ut float64, err error) {
	var dret [2]C.double
	buf, free := newErrBuf()
	defer free()
	rc := C.swe_utc_to_jd(C.int32_t(year), C.int32_t(month), C.int32_t(day),
		C.int32_t(hour), C.int32_t(minute), C.double(sec), C.int32_t(gregflag),
		&dret[0], buf)
	if rc == C.ERR {
		return 0, 0, errFrom(buf)
	}
	return float64(dret[0]), float64(dret[1]), nil
}

// JDUT1ToUTC converts a UT1 Julian Day to UTC calendar values.
func JDUT1ToUTC(jdUT float64, gregflag int) (year, month, day, hour, minute int, sec float64) {
	var y, mo, d, h, mi C.int32_t
	var s C.double
	C.swe_jdut1_to_utc(C.double(jdUT), C.int32_t(gregflag), &y, &mo, &d, &h, &mi, &s)
	return int(y), int(mo), int(d), int(h), int(mi), float64(s)
}

// DeltaT returns delta T for the given Julian Day, in days.
func DeltaT(jd float64, ephFlag int32) (float64, error) {
	buf, free := newErrBuf()
	defer free()
	v := C.swe_deltat_ex(C.double(jd), C.int32_t(ephFlag), buf)
	return float64(v), errFrom(buf)
}

// CalcResult holds the computed position of a celestial body.
//
// Longitude and Latitude are in degrees, Distance is in AU, and the speeds are
// in degrees or AU per day. When the equatorial flag is set, Longitude and
// Latitude carry right ascension and declination instead.
type CalcResult struct {
	Longitude     float64 `json:"longitude"`
	Latitude      float64 `json:"latitude"`
	Distance      float64 `json:"distance"`
	SpeedLong     float64 `json:"speed_longitude"`
	SpeedLat      float64 `json:"speed_latitude"`
	SpeedDistance float64 `json:"speed_distance"`

	// Flags are the flags Swiss Ephemeris actually applied, which can differ
	// from the ones requested: it falls back to the Moshier ephemeris when a
	// data file is missing.
	Flags int32 `json:"-"`

	// Warning carries a message the library returned alongside a successful
	// result.
	Warning string `json:"warning,omitempty"`
}

// CalcUT computes the position of body ipl at tjdUT, a UT1 Julian Day.
func CalcUT(tjdUT float64, ipl int, iflag int32) (CalcResult, error) {
	var xx [6]C.double
	buf, free := newErrBuf()
	defer free()
	rc := C.swe_calc_ut(C.double(tjdUT), C.int32_t(ipl), C.int32_t(iflag), &xx[0], buf)
	if rc < 0 {
		return CalcResult{}, errFrom(buf)
	}
	res := CalcResult{
		Longitude:     float64(xx[0]),
		Latitude:      float64(xx[1]),
		Distance:      float64(xx[2]),
		SpeedLong:     float64(xx[3]),
		SpeedLat:      float64(xx[4]),
		SpeedDistance: float64(xx[5]),
		Flags:         int32(rc),
	}
	// serr can hold a message even when rc >= 0. That is a warning, not a
	// failure.
	if msg := C.GoString(buf); msg != "" {
		res.Warning = msg
	}
	return res, nil
}

// PlanetName returns the name of a celestial body.
func PlanetName(ipl int) string {
	buf := (*C.char)(C.calloc(errBufSize, 1))
	defer C.free(unsafe.Pointer(buf))
	C.swe_get_planet_name(C.int32_t(ipl), buf)
	return C.GoString(buf)
}

// HousesResult holds the result of a house calculation.
type HousesResult struct {
	// Cusps holds the house cusps at indices 1..12; index 0 is unused. The
	// Gauquelin system (G) fills 1..36 instead.
	Cusps [37]float64

	// ASCMC holds, in order: Ascendant, MC, ARMC, Vertex, equatorial
	// Ascendant, co-Ascendant (Koch), co-Ascendant (Munkasey) and polar
	// Ascendant (Munkasey).
	ASCMC [10]float64
}

// HousesEx computes the house cusps for a moment and place. hsys is the house
// system letter: P for Placidus, K for Koch, W for whole sign, and so on.
func HousesEx(tjdUT float64, iflag int32, geolat, geolon float64, hsys byte) (HousesResult, error) {
	var res HousesResult
	var cusps [37]C.double
	var ascmc [10]C.double
	rc := C.swe_houses_ex(C.double(tjdUT), C.int32_t(iflag),
		C.double(geolat), C.double(geolon), C.int(hsys), &cusps[0], &ascmc[0])
	for i := range cusps {
		res.Cusps[i] = float64(cusps[i])
	}
	for i := range ascmc {
		res.ASCMC[i] = float64(ascmc[i])
	}
	if rc < 0 {
		return res, errors.New("swe_houses_ex: house system could not be computed")
	}
	return res, nil
}

// HousePos returns the house a position falls in as a fraction, so 8.53 means
// 53 percent of the way through the eighth house.
func HousePos(armc, geolat, eps float64, hsys byte, lon, lat float64) (float64, error) {
	buf, free := newErrBuf()
	defer free()
	xpin := [2]C.double{C.double(lon), C.double(lat)}
	v := C.swe_house_pos(C.double(armc), C.double(geolat), C.double(eps),
		C.int(hsys), &xpin[0], buf)
	if v <= 0 {
		return 0, errFrom(buf)
	}
	return float64(v), nil
}

// SetSidMode selects the ayanamsha used in sidereal mode.
func SetSidMode(sidMode int32, t0, ayanT0 float64) {
	C.swe_set_sid_mode(C.int32_t(sidMode), C.double(t0), C.double(ayanT0))
}

// GetAyanamsa returns the ayanamsha for the given moment, in degrees.
func GetAyanamsa(tjdUT float64, iflag int32) (float64, error) {
	var daya C.double
	buf, free := newErrBuf()
	defer free()
	rc := C.swe_get_ayanamsa_ex_ut(C.double(tjdUT), C.int32_t(iflag), &daya, buf)
	if rc < 0 {
		return 0, errFrom(buf)
	}
	return float64(daya), nil
}

// SetTopo sets the observer position used for topocentric calculations.
func SetTopo(geolon, geolat, geoalt float64) {
	C.swe_set_topo(C.double(geolon), C.double(geolat), C.double(geoalt))
}

// Pheno describes the appearance of a body: its phase, elongation and
// brightness.
type Pheno struct {
	PhaseAngle     float64 `json:"phase_angle"`
	PhaseIllumined float64 `json:"phase_illuminated"`
	Elongation     float64 `json:"elongation"`
	ApparentDiam   float64 `json:"apparent_diameter"`
	ApparentMag    float64 `json:"apparent_magnitude"`
}

// PhenoUT computes the appearance of a body.
func PhenoUT(tjdUT float64, ipl int, iflag int32) (Pheno, error) {
	var attr [20]C.double
	buf, free := newErrBuf()
	defer free()
	rc := C.swe_pheno_ut(C.double(tjdUT), C.int32_t(ipl), C.int32_t(iflag), &attr[0], buf)
	if rc < 0 {
		return Pheno{}, errFrom(buf)
	}
	return Pheno{
		PhaseAngle:     float64(attr[0]),
		PhaseIllumined: float64(attr[1]),
		Elongation:     float64(attr[2]),
		ApparentDiam:   float64(attr[3]),
		ApparentMag:    float64(attr[4]),
	}, nil
}

// RiseTrans searches for the next rise, set or meridian transit of a body.
// When starname is non-empty the search is performed for that fixed star.
// It reports found=false when the body never rises or sets in the search
// window, which happens at polar latitudes.
func RiseTrans(tjdUT float64, ipl int, starname string, epheflag, rsmi int32,
	geolon, geolat, geoalt, atpress, attemp float64) (jd float64, found bool, err error) {

	var cStar *C.char
	if starname != "" {
		cStar = (*C.char)(C.calloc(maxStarName, 1))
		defer C.free(unsafe.Pointer(cStar))
		s := C.CString(starname)
		C.strncpy(cStar, s, C.size_t(maxStarName-1))
		C.free(unsafe.Pointer(s))
	}

	geopos := [3]C.double{C.double(geolon), C.double(geolat), C.double(geoalt)}
	var tret C.double
	buf, free := newErrBuf()
	defer free()

	rc := C.swe_rise_trans(C.double(tjdUT), C.int32_t(ipl), cStar,
		C.int32_t(epheflag), C.int32_t(rsmi), &geopos[0],
		C.double(atpress), C.double(attemp), &tret, buf)
	switch {
	case rc == C.ERR:
		return 0, false, errFrom(buf)
	case rc == -2:
		return 0, false, nil
	}
	return float64(tret), true, nil
}

// NodesApsides holds the nodes and apsides of a planetary orbit.
type NodesApsides struct {
	AscendingNode  CalcResult `json:"ascending_node"`
	DescendingNode CalcResult `json:"descending_node"`
	Perihelion     CalcResult `json:"perihelion"`
	Aphelion       CalcResult `json:"aphelion"`
}

// NodApsUT computes the nodes and apsides of a planet.
func NodApsUT(tjdUT float64, ipl int, iflag, method int32) (NodesApsides, error) {
	var asc, desc, peri, aphe [6]C.double
	buf, free := newErrBuf()
	defer free()
	rc := C.swe_nod_aps_ut(C.double(tjdUT), C.int32_t(ipl), C.int32_t(iflag),
		C.int32_t(method), &asc[0], &desc[0], &peri[0], &aphe[0], buf)
	if rc < 0 {
		return NodesApsides{}, errFrom(buf)
	}
	conv := func(x [6]C.double) CalcResult {
		return CalcResult{
			Longitude: float64(x[0]), Latitude: float64(x[1]), Distance: float64(x[2]),
			SpeedLong: float64(x[3]), SpeedLat: float64(x[4]), SpeedDistance: float64(x[5]),
		}
	}
	return NodesApsides{
		AscendingNode:  conv(asc),
		DescendingNode: conv(desc),
		Perihelion:     conv(peri),
		Aphelion:       conv(aphe),
	}, nil
}

// FixStarUT computes the position of a fixed star. The returned name is the
// full star name as the library normalised it.
func FixStarUT(star string, tjdUT float64, iflag int32) (name string, res CalcResult, err error) {
	cStar := (*C.char)(C.calloc(maxStarName, 1))
	defer C.free(unsafe.Pointer(cStar))
	s := C.CString(star)
	C.strncpy(cStar, s, C.size_t(maxStarName-1))
	C.free(unsafe.Pointer(s))

	var xx [6]C.double
	buf, free := newErrBuf()
	defer free()
	rc := C.swe_fixstar2_ut(cStar, C.double(tjdUT), C.int32_t(iflag), &xx[0], buf)
	if rc < 0 {
		return "", CalcResult{}, errFrom(buf)
	}
	return C.GoString(cStar), CalcResult{
		Longitude: float64(xx[0]), Latitude: float64(xx[1]), Distance: float64(xx[2]),
		SpeedLong: float64(xx[3]), SpeedLat: float64(xx[4]), SpeedDistance: float64(xx[5]),
		Flags: int32(rc),
	}, nil
}

// HousesARMC computes house cusps from a sidereal time rather than from a
// moment, which is what a composite chart needs: there is no single instant to
// cast it for, only a midpoint between two.
func HousesARMC(armc, geolat, eps float64, hsys byte) (HousesResult, error) {
	var res HousesResult
	var cusps [37]C.double
	var ascmc [10]C.double

	rc := C.swe_houses_armc(C.double(armc), C.double(geolat), C.double(eps),
		C.int(hsys), &cusps[0], &ascmc[0])
	for i := range cusps {
		res.Cusps[i] = float64(cusps[i])
	}
	for i := range ascmc {
		res.ASCMC[i] = float64(ascmc[i])
	}
	if rc < 0 {
		return res, errors.New("swe_houses_armc: house system could not be computed")
	}
	return res, nil
}

// EclipseFlags are the bits returned alongside an eclipse, describing its
// kind.
const (
	EclCentral      = C.SE_ECL_CENTRAL
	EclNoncentral   = C.SE_ECL_NONCENTRAL
	EclTotal        = C.SE_ECL_TOTAL
	EclAnnular      = C.SE_ECL_ANNULAR
	EclPartial      = C.SE_ECL_PARTIAL
	EclAnnularTotal = C.SE_ECL_ANNULAR_TOTAL
	EclPenumbral    = C.SE_ECL_PENUMBRAL

	EclAllSolar = C.SE_ECL_ALLTYPES_SOLAR
	EclAllLunar = C.SE_ECL_ALLTYPES_LUNAR
)

// Eclipse is one eclipse, with the moments that define it.
//
// The meaning of each entry of Times depends on whether the eclipse is solar
// or lunar; the callers in package astro name them.
type Eclipse struct {
	// Kind carries the SE_ECL bits describing the eclipse.
	Kind int32
	// Times holds the moments Swiss Ephemeris returns, in Julian Days.
	Times [10]float64
}

// SolarEclipseWhenGlobal finds the next solar eclipse anywhere on Earth, at or
// after tjdStart. Set backward to search into the past instead.
func SolarEclipseWhenGlobal(tjdStart float64, iflag, ecltype int32, backward bool) (Eclipse, error) {
	var tret [10]C.double
	buf, free := newErrBuf()
	defer free()

	var back C.int32_t
	if backward {
		back = 1
	}
	rc := C.swe_sol_eclipse_when_glob(C.double(tjdStart), C.int32_t(iflag),
		C.int32_t(ecltype), &tret[0], back, buf)
	if rc == C.ERR {
		return Eclipse{}, errFrom(buf)
	}

	e := Eclipse{Kind: int32(rc)}
	for i := range tret {
		e.Times[i] = float64(tret[i])
	}
	return e, nil
}

// LunarEclipseWhen finds the next lunar eclipse at or after tjdStart. A lunar
// eclipse is visible from the whole night side at once, so it needs no place.
func LunarEclipseWhen(tjdStart float64, iflag, ecltype int32, backward bool) (Eclipse, error) {
	var tret [10]C.double
	buf, free := newErrBuf()
	defer free()

	var back C.int32_t
	if backward {
		back = 1
	}
	rc := C.swe_lun_eclipse_when(C.double(tjdStart), C.int32_t(iflag),
		C.int32_t(ecltype), &tret[0], back, buf)
	if rc == C.ERR {
		return Eclipse{}, errFrom(buf)
	}

	e := Eclipse{Kind: int32(rc)}
	for i := range tret {
		e.Times[i] = float64(tret[i])
	}
	return e, nil
}

// EclipseWhere is the place on Earth an eclipse is greatest, and how deep it
// is there.
type EclipseWhere struct {
	Longitude float64
	Latitude  float64
	// Magnitude is the fraction of the Sun's diameter covered.
	Magnitude float64
	// Obscuration is the fraction of the Sun's disc covered.
	Obscuration float64
}

// SolarEclipseWhere reports where on Earth a solar eclipse is greatest.
func SolarEclipseWhere(tjdUT float64, iflag int32) (EclipseWhere, error) {
	var geopos [20]C.double
	var attr [20]C.double
	buf, free := newErrBuf()
	defer free()

	rc := C.swe_sol_eclipse_where(C.double(tjdUT), C.int32_t(iflag),
		&geopos[0], &attr[0], buf)
	if rc < 0 {
		return EclipseWhere{}, errFrom(buf)
	}
	return EclipseWhere{
		Longitude:   float64(geopos[0]),
		Latitude:    float64(geopos[1]),
		Magnitude:   float64(attr[0]),
		Obscuration: float64(attr[2]),
	}, nil
}
