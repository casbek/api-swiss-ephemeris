package libswe

/*
#include "swephexp.h"
*/
import "C"

// Takvim bayraklari.
const (
	JulCal  = C.SE_JUL_CAL
	GregCal = C.SE_GREG_CAL
)

// Gok cisimleri (swe_calc_ut icin ipl degerleri).
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

	MeanNode = C.SE_MEAN_NODE // Ortalama Ay dugumu
	TrueNode = C.SE_TRUE_NODE // Gercek Ay dugumu
	MeanApog = C.SE_MEAN_APOG // Ortalama Lilith (Kara Ay)
	OscuApog = C.SE_OSCU_APOG // Oskulatif Lilith
	Earth    = C.SE_EARTH
	Chiron   = C.SE_CHIRON
	Pholus   = C.SE_PHOLUS
	Ceres    = C.SE_CERES
	Pallas   = C.SE_PALLAS
	Juno     = C.SE_JUNO
	Vesta    = C.SE_VESTA
	IntpApog = C.SE_INTP_APOG
	IntpPerg = C.SE_INTP_PERG

	// AstOffset, asteroid numarasina eklenerek ipl degeri uretilir.
	// Ornek: 433 numarali Eros icin AstOffset + 433.
	AstOffset = C.SE_AST_OFFSET
)

// Hesaplama bayraklari (iflag).
const (
	FlagJPLEph = C.SEFLG_JPLEPH
	FlagSwieph = C.SEFLG_SWIEPH // Swiss Ephemeris dosyalari (varsayilan)
	FlagMoseph = C.SEFLG_MOSEPH // Moshier: dosya gerektirmez, daha az hassas

	FlagHelctr  = C.SEFLG_HELCTR  // Heliosentrik
	FlagTruePos = C.SEFLG_TRUEPOS // Gercek (geometrik) konum
	FlagJ2000   = C.SEFLG_J2000
	FlagNoNut   = C.SEFLG_NONUT
	FlagSpeed   = C.SEFLG_SPEED // Yuksek hassasiyetli hiz (retro tespiti icin)

	FlagNoGDefl = C.SEFLG_NOGDEFL
	FlagNoAberr = C.SEFLG_NOABERR

	FlagEquatorial = C.SEFLG_EQUATORIAL // Sag acilim / deklinasyon
	FlagXYZ        = C.SEFLG_XYZ
	FlagRadians    = C.SEFLG_RADIANS
	FlagBaryctr    = C.SEFLG_BARYCTR
	FlagTopoctr    = C.SEFLG_TOPOCTR // Topocentrik (gozlemci merkezli)

	FlagTropical = C.SEFLG_TROPICAL // 0: varsayilan
	FlagSidereal = C.SEFLG_SIDEREAL // Vedik zodyak
)

// Ayanamsa (sidereal mod) secenekleri. Vedik astrolojide en yaygini Lahiri'dir.
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

// swe_rise_trans icin arama bayraklari.
const (
	CalcRise      = C.SE_CALC_RISE
	CalcSet       = C.SE_CALC_SET
	CalcMTransit  = C.SE_CALC_MTRANSIT // Ust gecis (meridyen)
	CalcITransit  = C.SE_CALC_ITRANSIT // Alt gecis
	BitDiscCenter = C.SE_BIT_DISC_CENTER
	BitNoRefract  = C.SE_BIT_NO_REFRACTION
	BitHinduRise  = C.SE_BIT_HINDU_RISING
)

// swe_nod_aps_ut icin yontem bayraklari.
const (
	NodbitMean     = C.SE_NODBIT_MEAN
	NodbitOscu     = C.SE_NODBIT_OSCU
	NodbitOscuBar  = C.SE_NODBIT_OSCU_BAR
	NodbitFocalPnt = C.SE_NODBIT_FOPOINT
)
