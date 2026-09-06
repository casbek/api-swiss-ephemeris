// Package swe, Swiss Ephemeris'i es zamanli kullanima uygun hale getiren
// guvenli sarmalayicidir. Ham cgo cagrilari icin internal/libswe'ye bakin.
package swe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/casbek/api-swiss-ephemeris/internal/libswe"
)

// Config, hesaplayicinin kurulum ayarlaridir.
type Config struct {
	// EphePath, .se1 ephemeris dosyalarinin bulundugu dizindir.
	EphePath string

	// Workers, ayrilacak OS thread sayisidir. 0 verilirse 1 kullanilir.
	// Windows'ta 1'den buyuk olamaz; bkz. newPool.
	Workers int
}

// Calculator, Swiss Ephemeris'e es zamanli guvenli erisim saglar.
// Tum metodlari birden fazla goroutine'den cagrilabilir.
type Calculator struct {
	p       *pool
	version string
}

// New, ephemeris dizinini dogrular ve hesaplayiciyi baslatir.
func New(cfg Config) (*Calculator, error) {
	if cfg.EphePath == "" {
		return nil, fmt.Errorf("swe: EphePath bos olamaz")
	}
	abs, err := filepath.Abs(cfg.EphePath)
	if err != nil {
		return nil, fmt.Errorf("swe: ephemeris yolu cozulemedi: %w", err)
	}
	if err := checkEpheDir(abs); err != nil {
		return nil, err
	}

	p, err := newPool(abs, cfg.Workers)
	if err != nil {
		return nil, err
	}

	c := &Calculator{p: p}
	// Surumu bir kez okuyup sakla; her istekte thread'e gitmeye gerek yok.
	if err := p.do(context.Background(), func() {
		c.version = libswe.Version()
	}); err != nil {
		p.close()
		return nil, err
	}
	return c, nil
}

// checkEpheDir, dizinin var oldugunu ve en az bir .se1 dosyasi
// icerdigini dogrular. Dosyalar eksikse Swiss Ephemeris sessizce
// Moshier'e duser ve sonuclar beklenenden az hassas olur; bunu
// baslangicta yakalamak calisma aninda tespit etmekten iyidir.
func checkEpheDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("swe: ephemeris dizini acilamadi (%s): %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("swe: ephemeris yolu bir dizin degil: %s", dir)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*.se1"))
	if err != nil {
		return fmt.Errorf("swe: ephemeris dizini taranamadi: %w", err)
	}
	if len(matches) == 0 {
		return fmt.Errorf("swe: %s icinde .se1 dosyasi yok; sepl_18.se1, semo_18.se1 ve seas_18.se1 gerekli", dir)
	}
	return nil
}

// Version, kullanilan Swiss Ephemeris surumunu dondurur.
func (c *Calculator) Version() string { return c.version }

// Close, worker thread'lerini durdurur ve ephemeris dosyalarini kapatir.
func (c *Calculator) Close() { c.p.close() }

// Do, fn'i Swiss Ephemeris'e ait bir thread uzerinde calistirir.
// Bir haritanin tum hesaplari tek bir Do cagrisinda yapilmalidir: boylece
// hepsi ayni thread uzerinde, tek seferde ve tutarli sekilde uretilir.
//
// Verilen Session yalnizca fn suresince gecerlidir; disari kacirilmamalidir.
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

// Session, bir Swiss Ephemeris thread'i uzerindeki hesaplama oturumudur.
// Yalnizca Calculator.Do icinden kullanilabilir.
type Session struct {
	valid bool
}

// Options, hesabin nasil yapilacagini belirler.
type Options struct {
	// Sidereal true ise Vedik (sidereal) zodyak kullanilir.
	Sidereal bool
	// Ayanamsa, Sidereal true iken kullanilacak ayanamsa numarasidir
	// (libswe.Sidm* sabitleri). Varsayilan Lahiri'dir.
	Ayanamsa int32

	// Topocentric true ise konumlar gozlemci merkezli hesaplanir.
	// Ay icin fark yaklasik 1 dereceye kadar cikabilir.
	Topocentric bool
	Latitude    float64
	Longitude   float64
	AltitudeM   float64

	// Heliocentric true ise Gunes merkezli konum hesaplanir.
	Heliocentric bool

	// Equatorial true ise ekliptik yerine ekvatoral koordinat
	// (sag acilim / deklinasyon) doner.
	Equatorial bool

	// TruePositions true ise isik zamani duzeltmesi yapilmamis
	// gercek geometrik konum doner.
	TruePositions bool

	// NoNutation true ise nutasyon uygulanmaz (ortalama ekinoks).
	NoNutation bool
}

// Flags, secenekleri Swiss Ephemeris iflag degerine cevirir.
func (o Options) Flags() int32 {
	// SEFLG_SPEED her zaman aciktir: retrograd tespiti hiz degerine dayanir
	// ve maliyeti ihmal edilebilir.
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

// apply, thread'e ait ayarlari bu istek icin gecerli hale getirir ve
// kullanilacak iflag degerini dondurur.
//
// Worker thread'leri istekler arasinda yeniden kullanildigi icin bu ayarlar
// HER hesaptan once yazilmalidir; aksi halde onceki istegin ayanamsasi veya
// gozlemci konumu bir sonrakine sizar.
func (s *Session) apply(o Options) int32 {
	if !s.valid {
		panic("swe: Session, Calculator.Do disinda kullanildi")
	}

	ayan := o.Ayanamsa
	if !o.Sidereal {
		// Sidereal degilken de sifirla: thread'de kalan onceki deger
		// ayanamsa sorgulayan cagrilari etkilemesin.
		ayan = libswe.SidmFaganBradley
	}
	libswe.SetSidMode(ayan, 0, 0)

	if o.Topocentric {
		libswe.SetTopo(o.Longitude, o.Latitude, o.AltitudeM)
	}
	return o.Flags()
}

// Calc, tjdUT aninda ipl numarali gok cisminin konumunu hesaplar.
func (s *Session) Calc(tjdUT float64, ipl int, o Options) (libswe.CalcResult, error) {
	iflag := s.apply(o)
	res, err := libswe.CalcUT(tjdUT, ipl, iflag)
	if err != nil {
		return res, fmt.Errorf("swe: %s hesaplanamadi: %w", libswe.PlanetName(ipl), err)
	}
	return res, nil
}

// Houses, verilen an ve konum icin ev sistemini hesaplar.
func (s *Session) Houses(tjdUT, lat, lon float64, hsys byte, o Options) (libswe.HousesResult, error) {
	iflag := s.apply(o)
	return libswe.HousesEx(tjdUT, iflag, lat, lon, hsys)
}

// HousePos, bir konumun kacinci evde oldugunu ondalikli olarak dondurur.
func (s *Session) HousePos(armc, lat, eps float64, hsys byte, lon, plat float64) (float64, error) {
	return libswe.HousePos(armc, lat, eps, hsys, lon, plat)
}

// Ayanamsa, verilen an icin ayanamsa degerini derece olarak dondurur.
func (s *Session) Ayanamsa(tjdUT float64, o Options) (float64, error) {
	iflag := s.apply(o)
	return libswe.GetAyanamsa(tjdUT, iflag)
}

// Pheno, gok cisminin faz ve parlaklik bilgilerini dondurur.
func (s *Session) Pheno(tjdUT float64, ipl int, o Options) (libswe.Pheno, error) {
	iflag := s.apply(o)
	return libswe.PhenoUT(tjdUT, ipl, iflag)
}

// RiseTrans, dogus/batis/gecis anini arar. rsmi icin libswe.CalcRise gibi
// sabitleri kullanin.
func (s *Session) RiseTrans(tjdUT float64, ipl int, rsmi int32, lat, lon, altM float64, o Options) (jd float64, found bool, err error) {
	iflag := s.apply(o)
	// atpress=0 verildiginde kutuphane basinci yukseklikten kendisi hesaplar.
	return libswe.RiseTrans(tjdUT, ipl, "", iflag, rsmi, lon, lat, altM, 0, 15)
}

// NodesApsides, gezegenin dugum ve apsis noktalarini dondurur.
func (s *Session) NodesApsides(tjdUT float64, ipl int, method int32, o Options) (libswe.NodesApsides, error) {
	iflag := s.apply(o)
	return libswe.NodApsUT(tjdUT, ipl, iflag, method)
}

// FixStar, sabit yildizin konumunu hesaplar.
func (s *Session) FixStar(name string, tjdUT float64, o Options) (string, libswe.CalcResult, error) {
	iflag := s.apply(o)
	return libswe.FixStarUT(name, tjdUT, iflag)
}

// DeltaT, verilen Julian Day icin delta T degerini gun cinsinden dondurur.
func (s *Session) DeltaT(jd float64) (float64, error) {
	return libswe.DeltaT(jd, libswe.FlagSwieph)
}

// UTCToJD, UTC tarih/saatini Julian Day'e cevirir (artik saniyeler dahil).
// Bu fonksiyon Swiss Ephemeris global durumuna dokunmaz, oturum disinda da
// cagrilabilir; kolaylik olsun diye burada da sunulur.
func UTCToJD(year, month, day, hour, minute int, sec float64) (et, ut float64, err error) {
	return libswe.UTCToJD(year, month, day, hour, minute, sec, libswe.GregCal)
}

// JDToUTC, UT1 Julian Day degerini UTC takvim degerlerine cevirir.
func JDToUTC(jdUT float64) (year, month, day, hour, minute int, sec float64) {
	return libswe.JDUT1ToUTC(jdUT, libswe.GregCal)
}
