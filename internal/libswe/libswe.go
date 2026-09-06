// Package libswe, Swiss Ephemeris 2.10.03 C kutuphanesinin ham cgo sarmalayicisidir.
//
// DIKKAT: Bu paketteki fonksiyonlar dogrudan cagrilmamalidir. Swiss Ephemeris
// global durum tutar (Linux/GCC uzerinde thread-local, Windows'ta paylasimli),
// bu yuzden tum cagrilar internal/swe paketindeki worker havuzu uzerinden
// yapilmalidir. Bkz. internal/swe/pool.go
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

// errBufSize, Swiss Ephemeris'in AS_MAXCH sabitiyle ayni olmalidir.
const errBufSize = 256

// maxStarName, swe_fixstar2_ut icin SE_MAX_STNAME + 1 boyutundadir.
const maxStarName = 256

// newErrBuf, C tarafina verilecek hata tamponunu ve serbest birakma
// fonksiyonunu dondurur.
func newErrBuf() (*C.char, func()) {
	buf := (*C.char)(C.calloc(errBufSize, 1))
	return buf, func() { C.free(unsafe.Pointer(buf)) }
}

// errFrom, C hata tamponunu Go hatasina cevirir. Tampon bossa nil doner.
func errFrom(buf *C.char) error {
	msg := C.GoString(buf)
	if msg == "" {
		return nil
	}
	return errors.New(msg)
}

// SetEphePath, ephemeris dosyalarinin bulundugu dizini ayarlar.
// Her OS thread'i icin ayri ayri cagrilmalidir (TLS).
func SetEphePath(path string) {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	C.swe_set_ephe_path(cPath)
}

// Close, acik ephemeris dosyalarini kapatir ve bellegi serbest birakir.
func Close() { C.swe_close() }

// Version, kutuphane surumunu dondurur.
func Version() string {
	buf := (*C.char)(C.calloc(errBufSize, 1))
	defer C.free(unsafe.Pointer(buf))
	C.swe_version(buf)
	return C.GoString(buf)
}

// Julday, takvim tarihini Julian Day'e cevirir. hour ondalikli saattir (UT).
// gregflag icin GregCal veya JulCal kullanin.
func Julday(year, month, day int, hour float64, gregflag int) float64 {
	return float64(C.swe_julday(C.int(year), C.int(month), C.int(day),
		C.double(hour), C.int(gregflag)))
}

// Revjul, Julian Day'i takvim tarihine cevirir.
func Revjul(jd float64, gregflag int) (year, month, day int, hour float64) {
	var y, m, d C.int
	var h C.double
	C.swe_revjul(C.double(jd), C.int(gregflag), &y, &m, &d, &h)
	return int(y), int(m), int(d), float64(h)
}

// UTCToJD, UTC tarih/saatini Julian Day'e cevirir. Artik saniyeleri dikkate alir.
// Donen degerlerden et TT (Terrestrial Time), ut ise UT1 cinsindendir.
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

// JDUT1ToUTC, UT1 cinsinden Julian Day'i UTC takvim degerlerine cevirir.
func JDUT1ToUTC(jdUT float64, gregflag int) (year, month, day, hour, minute int, sec float64) {
	var y, mo, d, h, mi C.int32_t
	var s C.double
	C.swe_jdut1_to_utc(C.double(jdUT), C.int32_t(gregflag), &y, &mo, &d, &h, &mi, &s)
	return int(y), int(mo), int(d), int(h), int(mi), float64(s)
}

// DeltaT, verilen Julian Day icin delta T degerini (gun cinsinden) dondurur.
func DeltaT(jd float64, ephFlag int32) (float64, error) {
	buf, free := newErrBuf()
	defer free()
	v := C.swe_deltat_ex(C.double(jd), C.int32_t(ephFlag), buf)
	return float64(v), errFrom(buf)
}

// CalcResult, bir gok cismi hesabinin sonucudur.
// Longitude/Latitude derece, Distance AU, hiz degerleri derece veya AU/gun
// cinsindendir. Ekvatoral bayrak verildiginde Longitude/Latitude yerine
// sag acilim/deklinasyon degerleri doner.
type CalcResult struct {
	Longitude     float64 `json:"longitude"`
	Latitude      float64 `json:"latitude"`
	Distance      float64 `json:"distance"`
	SpeedLong     float64 `json:"speed_longitude"`
	SpeedLat      float64 `json:"speed_latitude"`
	SpeedDistance float64 `json:"speed_distance"`

	// Flags, Swiss Ephemeris'in gercekte uyguladigi bayraklardir. Istenen
	// bayraklardan farkli olabilir (orn. ephemeris dosyasi bulunamayip
	// Moshier'e dusulmesi).
	Flags int32 `json:"-"`

	// Warning, hesap basarili oldugu halde kutuphanenin dondurdugu uyaridir.
	Warning string `json:"warning,omitempty"`
}

// CalcUT, tjdUT (UT1 Julian Day) aninda ipl numarali gok cisminin
// konumunu hesaplar.
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
	// rc >= 0 iken serr dolu olabilir; bu bir uyaridir, hata degil.
	if msg := C.GoString(buf); msg != "" {
		res.Warning = msg
	}
	return res, nil
}

// PlanetName, gok cisminin adini dondurur.
func PlanetName(ipl int) string {
	buf := (*C.char)(C.calloc(errBufSize, 1))
	defer C.free(unsafe.Pointer(buf))
	C.swe_get_planet_name(C.int32_t(ipl), buf)
	return C.GoString(buf)
}

// HousesResult, ev hesabinin sonucudur.
type HousesResult struct {
	// Cusps, 1..12 indislerinde ev baslangiclarini tutar. 0. indis kullanilmaz.
	// Gauquelin sistemi (G) icin 1..36 doludur.
	Cusps [37]float64

	// ASCMC sirasi: Asc, MC, ARMC, Vertex, EquatorialAsc,
	// CoAsc(Koch), CoAsc(Munkasey), PolarAsc(Munkasey)
	ASCMC [10]float64
}

// HousesEx, verilen an ve konum icin ev sistemini hesaplar.
// hsys, ev sistemi harfidir (P = Placidus, K = Koch, W = Whole Sign ...).
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
		return res, errors.New("swe_houses_ex: ev sistemi hesaplanamadi")
	}
	return res, nil
}

// HousePos, bir gok cisminin hangi evde oldugunu ondalikli olarak dondurur
// (orn. 8.53 => 8. evin yuzde 53'unde).
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

// SetSidMode, sidereal (Vedik) mod icin ayanamsa secer.
func SetSidMode(sidMode int32, t0, ayanT0 float64) {
	C.swe_set_sid_mode(C.int32_t(sidMode), C.double(t0), C.double(ayanT0))
}

// GetAyanamsa, verilen an icin ayanamsa degerini derece olarak dondurur.
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

// SetTopo, topocentrik hesaplar icin gozlemci konumunu ayarlar.
func SetTopo(geolon, geolat, geoalt float64) {
	C.swe_set_topo(C.double(geolon), C.double(geolat), C.double(geoalt))
}

// Pheno, gok cisminin faz acisi, aydinlanma orani ve parlakligi gibi
// gorunum bilgileridir.
type Pheno struct {
	PhaseAngle     float64 `json:"phase_angle"`
	PhaseIllumined float64 `json:"phase_illuminated"`
	Elongation     float64 `json:"elongation"`
	ApparentDiam   float64 `json:"apparent_diameter"`
	ApparentMag    float64 `json:"apparent_magnitude"`
}

// PhenoUT, gok cisminin gorunum bilgilerini hesaplar.
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

// RiseTrans, bir gok cisminin dogus/batis/gecis anini arar.
// starname bos degilse hesap sabit yildiz icin yapilir.
// Cisim aranan aralikta hic dogmuyor/batmiyorsa found=false doner.
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
		// Cisim kutup bolgesinde hic dogmuyor/batmiyor.
		return 0, false, nil
	}
	return float64(tret), true, nil
}

// NodesApsides, bir gezegenin dugum ve apsis noktalaridir.
type NodesApsides struct {
	AscendingNode  CalcResult `json:"ascending_node"`
	DescendingNode CalcResult `json:"descending_node"`
	Perihelion     CalcResult `json:"perihelion"`
	Aphelion       CalcResult `json:"aphelion"`
}

// NodApsUT, gezegenin dugum ve apsis noktalarini hesaplar.
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

// FixStarUT, sabit yildizin konumunu hesaplar. Donen ad, kutuphanenin
// normallestirdigi tam yildiz adidir.
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
