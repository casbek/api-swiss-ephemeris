package swe

import (
	"context"
	"math"
	"sync"
	"testing"

	"github.com/casbek/api-swiss-ephemeris/internal/libswe"
)

// refJD, 15.06.1990 14:30:00 UT anina karsilik gelen Julian Day degeridir.
const refJD = 2448058.104166667

// Referans degerler, Swiss Ephemeris 2.10.03 ile birlikte gelen swetest
// araciyla uretilmistir:
//
//	swetest -edir<ephe> -b15.06.1990 -ut14:30 -p0123456789 -fPls -eswe
//
// Bu sayede kendi cgo sarmalayicimizin kutuphaneyi dogru cagirdigi,
// ephemeris dosyalarinin gercekten okundugu (Moshier'e dusulmedigi) ve
// bayraklarin beklendigi gibi uygulandigi dogrulanir.
var refPlanets = map[int]float64{
	libswe.Sun:     84.2290447,
	libswe.Moon:    346.7518340,
	libswe.Mercury: 65.8696941,
	libswe.Venus:   48.9001012,
	libswe.Mars:    11.1161937,
	libswe.Jupiter: 105.9120560,
	libswe.Saturn:  294.0258212,
	libswe.Uranus:  278.1611546,
	libswe.Neptune: 283.7144691,
	libswe.Pluto:   225.3994063,
}

// refCusps, ayni an ve Istanbul (28.9784 D, 41.0082 K) icin Placidus
// ev baslangiclaridir. Indis 1..12.
var refCusps = [13]float64{
	0,
	227.1761062, 256.9296887, 291.5988665, 327.9146197,
	0.0896658, 26.0679902, 47.1761062, 76.9296887,
	111.5988665, 147.9146197, 180.0896658, 206.0679902,
}

const (
	refAsc  = 227.1761062
	refMC   = 147.9146197
	refARMC = 150.0926041
	// tolerance, swetest'in yazdirdigi 7 ondalik basamaga karsilik gelir.
	tolerance = 1e-6
)

func newTestCalculator(t *testing.T) *Calculator {
	t.Helper()
	c, err := New(Config{EphePath: "../../ephe"})
	if err != nil {
		t.Fatalf("hesaplayici baslatilamadi: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestVersion(t *testing.T) {
	c := newTestCalculator(t)
	if got := c.Version(); got != "2.10.03" {
		t.Errorf("Version() = %q, beklenen 2.10.03", got)
	}
}

// TestPlanetPositionsMatchSwetest, gezegen boylamlarinin referans araciyla
// birebir ortustugunu dogrular.
func TestPlanetPositionsMatchSwetest(t *testing.T) {
	c := newTestCalculator(t)

	err := c.Do(context.Background(), func(s *Session) error {
		for ipl, want := range refPlanets {
			res, err := s.Calc(refJD, ipl, Options{})
			if err != nil {
				return err
			}
			if res.Warning != "" {
				// En sik uyari, .se1 dosyasi bulunamayip Moshier'e
				// dusuldugudur; bu sessizce hassasiyet kaybettirir.
				t.Errorf("%s: beklenmeyen uyari: %s", libswe.PlanetName(ipl), res.Warning)
			}
			if diff := math.Abs(res.Longitude - want); diff > tolerance {
				t.Errorf("%s boylam = %.7f, beklenen %.7f (fark %.2e)",
					libswe.PlanetName(ipl), res.Longitude, want, diff)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("hesap basarisiz: %v", err)
	}
}

// TestSpeedFlagDetectsRetrograde, SEFLG_SPEED'in gercekten uygulandigini
// ve retrograd tespitinin calistigini dogrular. Bu tarihte Saturn, Uranus,
// Neptune ve Pluto geri harekettedir.
func TestSpeedFlagDetectsRetrograde(t *testing.T) {
	c := newTestCalculator(t)

	retro := map[int]bool{
		libswe.Sun: false, libswe.Moon: false, libswe.Mercury: false,
		libswe.Saturn: true, libswe.Uranus: true, libswe.Neptune: true,
		libswe.Pluto: true,
	}

	err := c.Do(context.Background(), func(s *Session) error {
		for ipl, want := range retro {
			res, err := s.Calc(refJD, ipl, Options{})
			if err != nil {
				return err
			}
			if got := res.SpeedLong < 0; got != want {
				t.Errorf("%s retrograd = %v (hiz %.6f), beklenen %v",
					libswe.PlanetName(ipl), got, res.SpeedLong, want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("hesap basarisiz: %v", err)
	}
}

func TestHousesPlacidus(t *testing.T) {
	c := newTestCalculator(t)

	err := c.Do(context.Background(), func(s *Session) error {
		h, err := s.Houses(refJD, 41.0082, 28.9784, 'P', Options{})
		if err != nil {
			return err
		}
		for i := 1; i <= 12; i++ {
			if diff := math.Abs(h.Cusps[i] - refCusps[i]); diff > tolerance {
				t.Errorf("ev %d = %.7f, beklenen %.7f", i, h.Cusps[i], refCusps[i])
			}
		}
		if diff := math.Abs(h.ASCMC[0] - refAsc); diff > tolerance {
			t.Errorf("Ascendant = %.7f, beklenen %.7f", h.ASCMC[0], refAsc)
		}
		if diff := math.Abs(h.ASCMC[1] - refMC); diff > tolerance {
			t.Errorf("MC = %.7f, beklenen %.7f", h.ASCMC[1], refMC)
		}
		if diff := math.Abs(h.ASCMC[2] - refARMC); diff > tolerance {
			t.Errorf("ARMC = %.7f, beklenen %.7f", h.ASCMC[2], refARMC)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ev hesabi basarisiz: %v", err)
	}
}

// TestSiderealDiffersFromTropical, sidereal modun gercekten devreye
// girdigini ve farkin ayanamsa kadar oldugunu dogrular.
func TestSiderealDiffersFromTropical(t *testing.T) {
	c := newTestCalculator(t)

	err := c.Do(context.Background(), func(s *Session) error {
		trop, err := s.Calc(refJD, libswe.Sun, Options{})
		if err != nil {
			return err
		}
		sidOpts := Options{Sidereal: true, Ayanamsa: libswe.SidmLahiri}
		sid, err := s.Calc(refJD, libswe.Sun, sidOpts)
		if err != nil {
			return err
		}
		ayan, err := s.Ayanamsa(refJD, sidOpts)
		if err != nil {
			return err
		}
		// 1990'da Lahiri ayanamsasi yaklasik 23.7 derecedir.
		if ayan < 23 || ayan > 24.5 {
			t.Errorf("Lahiri ayanamsa = %.4f, 23-24.5 araliginda beklenirdi", ayan)
		}
		got := math.Mod(trop.Longitude-sid.Longitude+360, 360)
		if diff := math.Abs(got - ayan); diff > 1e-6 {
			t.Errorf("tropikal-sidereal farki = %.7f, ayanamsa %.7f", got, ayan)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("sidereal hesap basarisiz: %v", err)
	}
}

// TestOptionsDoNotLeakBetweenCalls, worker thread'lerinin istekler arasinda
// yeniden kullanilmasinin durum sizdirmadigini dogrular. Once sidereal bir
// hesap yapilir, ardindan tropikal hesabin referans degeri vermesi beklenir.
func TestOptionsDoNotLeakBetweenCalls(t *testing.T) {
	c := newTestCalculator(t)

	err := c.Do(context.Background(), func(s *Session) error {
		if _, err := s.Calc(refJD, libswe.Sun, Options{
			Sidereal: true, Ayanamsa: libswe.SidmKrishnamurti,
		}); err != nil {
			return err
		}
		if _, err := s.Calc(refJD, libswe.Moon, Options{
			Topocentric: true, Latitude: 41.0082, Longitude: 28.9784,
		}); err != nil {
			return err
		}
		res, err := s.Calc(refJD, libswe.Sun, Options{})
		if err != nil {
			return err
		}
		if diff := math.Abs(res.Longitude - refPlanets[libswe.Sun]); diff > tolerance {
			t.Errorf("onceki istegin ayarlari sizdi: Gunes = %.7f, beklenen %.7f",
				res.Longitude, refPlanets[libswe.Sun])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("hesap basarisiz: %v", err)
	}
}

// TestConcurrentCalls, es zamanli isteklerin ayni sonucu urettigini dogrular.
// -race ile calistirildiginda ayrica veri yarisi olmadigini gosterir.
func TestConcurrentCalls(t *testing.T) {
	c := newTestCalculator(t)

	const goroutines = 32
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := c.Do(context.Background(), func(s *Session) error {
				res, err := s.Calc(refJD, libswe.Sun, Options{})
				if err != nil {
					return err
				}
				if diff := math.Abs(res.Longitude - refPlanets[libswe.Sun]); diff > tolerance {
					t.Errorf("es zamanli hesap sapti: %.7f", res.Longitude)
				}
				return nil
			})
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("es zamanli hesap hatasi: %v", err)
	}
}

// TestSessionEscapePanics, Session'in Do disinda kullanilmasinin sessizce
// yanlis sonuc uretmek yerine hemen fark edilmesini dogrular.
func TestSessionEscapePanics(t *testing.T) {
	c := newTestCalculator(t)

	var escaped *Session
	if err := c.Do(context.Background(), func(s *Session) error {
		escaped = s
		return nil
	}); err != nil {
		t.Fatalf("Do basarisiz: %v", err)
	}

	defer func() {
		if recover() == nil {
			t.Error("kacirilan Session kullaniminda panic bekleniyordu")
		}
	}()
	_, _ = escaped.Calc(refJD, libswe.Sun, Options{})
}

func TestMissingEphePathFails(t *testing.T) {
	if _, err := New(Config{EphePath: "../../ephe-yok"}); err == nil {
		t.Error("olmayan ephemeris dizini icin hata bekleniyordu")
	}
}

func TestUTCToJD(t *testing.T) {
	_, ut, err := UTCToJD(1990, 6, 15, 14, 30, 0)
	if err != nil {
		t.Fatalf("UTCToJD basarisiz: %v", err)
	}
	// swe_utc_to_jd artik saniyeleri hesaba kattigi icin sonuc, ham
	// swe_julday degerinden birkac saniye farklidir.
	if diff := math.Abs(ut - refJD); diff > 1e-4 {
		t.Errorf("UT = %.9f, referans %.9f (fark %.2e gun)", ut, refJD, diff)
	}
}

// BenchmarkNatalChart, tam bir natal harita hesabinin maliyetini olcer:
// 10 gezegen + 12 ev + aciklar icin gereken temel Swiss Ephemeris cagrilari.
func BenchmarkNatalChart(b *testing.B) {
	c, err := New(Config{EphePath: "../../ephe"})
	if err != nil {
		b.Fatalf("hesaplayici baslatilamadi: %v", err)
	}
	defer c.Close()

	bodies := []int{
		libswe.Sun, libswe.Moon, libswe.Mercury, libswe.Venus, libswe.Mars,
		libswe.Jupiter, libswe.Saturn, libswe.Uranus, libswe.Neptune, libswe.Pluto,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := c.Do(context.Background(), func(s *Session) error {
			for _, ipl := range bodies {
				if _, err := s.Calc(refJD, ipl, Options{}); err != nil {
					return err
				}
			}
			_, err := s.Houses(refJD, 41.0082, 28.9784, 'P', Options{})
			return err
		})
		if err != nil {
			b.Fatalf("hesap basarisiz: %v", err)
		}
	}
}

// BenchmarkNatalChartParallel, es zamanli istek yuku altindaki davranisi olcer.
func BenchmarkNatalChartParallel(b *testing.B) {
	c, err := New(Config{EphePath: "../../ephe"})
	if err != nil {
		b.Fatalf("hesaplayici baslatilamadi: %v", err)
	}
	defer c.Close()

	bodies := []int{
		libswe.Sun, libswe.Moon, libswe.Mercury, libswe.Venus, libswe.Mars,
		libswe.Jupiter, libswe.Saturn, libswe.Uranus, libswe.Neptune, libswe.Pluto,
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			err := c.Do(context.Background(), func(s *Session) error {
				for _, ipl := range bodies {
					if _, err := s.Calc(refJD, ipl, Options{}); err != nil {
						return err
					}
				}
				_, err := s.Houses(refJD, 41.0082, 28.9784, 'P', Options{})
				return err
			})
			if err != nil {
				b.Fatalf("hesap basarisiz: %v", err)
			}
		}
	})
}
