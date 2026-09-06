package libswe

/*
#include "swephexp.h"
*/
import "C"

// Calendar flags.
const (
	JulCal  = C.SE_JUL_CAL
	GregCal = C.SE_GREG_CAL
)

// Celestial bodies, the ipl values accepted by CalcUT.
const (
	EclNut = C.SE_ECL_NUT

	Sun     = C.SE_SUN
	Moon    = C.SE_MOON
	Mercury = C.SE_MERCURY
	Venus   = C.SE_VENUS
	Mars    = C.SE_MARS
	Jupiter = C.SE_JUPITER
	Saturn  = C.SE_SATURN
	Uranus  = C.SE_URANUS
	Neptune = C.SE_NEPTUNE
	Pluto   = C.SE_PLUTO

	MeanNode = C.SE_MEAN_NODE // mean lunar node
	TrueNode = C.SE_TRUE_NODE // true lunar node
	MeanApog = C.SE_MEAN_APOG // mean lunar apogee, the black moon Lilith
	OscuApog = C.SE_OSCU_APOG // osculating lunar apogee
	Earth    = C.SE_EARTH
	Chiron   = C.SE_CHIRON
	Pholus   = C.SE_PHOLUS
	Ceres    = C.SE_CERES
	Pallas   = C.SE_PALLAS
	Juno     = C.SE_JUNO
	Vesta    = C.SE_VESTA
	IntpApog = C.SE_INTP_APOG
	IntpPerg = C.SE_INTP_PERG

	// AstOffset is added to an asteroid number to form its ipl value, so
	// asteroid 433 Eros is AstOffset + 433.
	AstOffset = C.SE_AST_OFFSET
)

// Calculation flags, combined into the iflag argument.
const (
	FlagJPLEph = C.SEFLG_JPLEPH
	FlagSwieph = C.SEFLG_SWIEPH // Swiss Ephemeris data files, the default
	FlagMoseph = C.SEFLG_MOSEPH // Moshier: needs no data files, less precise

	FlagHelctr  = C.SEFLG_HELCTR  // heliocentric
	FlagTruePos = C.SEFLG_TRUEPOS // true geometric position
	FlagJ2000   = C.SEFLG_J2000
	FlagNoNut   = C.SEFLG_NONUT
	FlagSpeed   = C.SEFLG_SPEED // high precision speed, needed to detect retrogrades

	FlagNoGDefl = C.SEFLG_NOGDEFL
	FlagNoAberr = C.SEFLG_NOABERR

	FlagEquatorial = C.SEFLG_EQUATORIAL // right ascension and declination
	FlagXYZ        = C.SEFLG_XYZ
	FlagRadians    = C.SEFLG_RADIANS
	FlagBaryctr    = C.SEFLG_BARYCTR
	FlagTopoctr    = C.SEFLG_TOPOCTR // topocentric, seen from the observer

	FlagTropical = C.SEFLG_TROPICAL // zero, the default
	FlagSidereal = C.SEFLG_SIDEREAL // sidereal zodiac
)

// Ayanamshas available in sidereal mode. Lahiri is the most widely used in
// Vedic astrology.
const (
	SidmFaganBradley = C.SE_SIDM_FAGAN_BRADLEY
	SidmLahiri       = C.SE_SIDM_LAHIRI
	SidmDeluce       = C.SE_SIDM_DELUCE
	SidmRaman        = C.SE_SIDM_RAMAN
	SidmKrishnamurti = C.SE_SIDM_KRISHNAMURTI
	SidmYukteshwar   = C.SE_SIDM_YUKTESHWAR
	SidmJNBhasin     = C.SE_SIDM_JN_BHASIN
	SidmTrueCitra    = C.SE_SIDM_TRUE_CITRA
	SidmTrueRevati   = C.SE_SIDM_TRUE_REVATI
	SidmTruePushya   = C.SE_SIDM_TRUE_PUSHYA
	SidmLahiriICRC   = C.SE_SIDM_LAHIRI_ICRC
	SidmUser         = C.SE_SIDM_USER
)

// Search flags for RiseTrans.
const (
	CalcRise      = C.SE_CALC_RISE
	CalcSet       = C.SE_CALC_SET
	CalcMTransit  = C.SE_CALC_MTRANSIT // upper meridian transit
	CalcITransit  = C.SE_CALC_ITRANSIT // lower meridian transit
	BitDiscCenter = C.SE_BIT_DISC_CENTER
	BitNoRefract  = C.SE_BIT_NO_REFRACTION
	BitHinduRise  = C.SE_BIT_HINDU_RISING
)

// Method flags for NodApsUT.
const (
	NodbitMean     = C.SE_NODBIT_MEAN
	NodbitOscu     = C.SE_NODBIT_OSCU
	NodbitOscuBar  = C.SE_NODBIT_OSCU_BAR
	NodbitFocalPnt = C.SE_NODBIT_FOPOINT
)
