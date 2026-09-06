package swe

import (
	"context"
	"math"
	"sync"
	"testing"

	"github.com/casbek/api-swiss-ephemeris/internal/libswe"
)

// refJD is the Julian Day for 1990-06-15 14:30:00 UT.
const refJD = 2448058.104166667

// The reference values below were produced with swetest, the command line tool
// shipped with Swiss Ephemeris 2.10.03:
//
//	swetest -edir<ephe> -b15.06.1990 -ut14:30 -p0123456789 -fPls -eswe
//
// Comparing against it proves that our cgo bindings call the library
// correctly, that the ephemeris files are really being read rather than
// falling back to Moshier, and that the flags are applied as intended.
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

// refCusps holds the Placidus house cusps for the same moment at Istanbul
// (28.9784 E, 41.0082 N), at indices 1..12.
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
	// tolerance matches the seven decimal places swetest prints.
	tolerance = 1e-6
)

func newTestCalculator(t *testing.T) *Calculator {
	t.Helper()
	c, err := New(Config{EphePath: "../../ephe"})
	if err != nil {
		t.Fatalf("could not start calculator: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestVersion(t *testing.T) {
	c := newTestCalculator(t)
	if got := c.Version(); got != "2.10.03" {
		t.Errorf("Version() = %q, want 2.10.03", got)
	}
}

// TestPlanetPositionsMatchSwetest checks the planetary longitudes against the
// reference tool.
func TestPlanetPositionsMatchSwetest(t *testing.T) {
	c := newTestCalculator(t)

	err := c.Do(context.Background(), func(s *Session) error {
		for ipl, want := range refPlanets {
			res, err := s.Calc(refJD, ipl, Options{})
			if err != nil {
				return err
			}
			if res.Warning != "" {
				// The most common warning is that a .se1 file was not
				// found and Moshier was used instead, which silently
				// costs precision.
				t.Errorf("%s: unexpected warning: %s", libswe.PlanetName(ipl), res.Warning)
			}
			if diff := math.Abs(res.Longitude - want); diff > tolerance {
				t.Errorf("%s longitude = %.7f, want %.7f (off by %.2e)",
					libswe.PlanetName(ipl), res.Longitude, want, diff)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("calculation failed: %v", err)
	}
}

// TestSpeedFlagDetectsRetrograde checks that SEFLG_SPEED is really applied and
// that retrograde motion is detected. On this date Saturn, Uranus, Neptune and
// Pluto are retrograde.
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
				t.Errorf("%s retrograde = %v (speed %.6f), want %v",
					libswe.PlanetName(ipl), got, res.SpeedLong, want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("calculation failed: %v", err)
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
				t.Errorf("house %d = %.7f, want %.7f", i, h.Cusps[i], refCusps[i])
			}
		}
		if diff := math.Abs(h.ASCMC[0] - refAsc); diff > tolerance {
			t.Errorf("Ascendant = %.7f, want %.7f", h.ASCMC[0], refAsc)
		}
		if diff := math.Abs(h.ASCMC[1] - refMC); diff > tolerance {
			t.Errorf("MC = %.7f, want %.7f", h.ASCMC[1], refMC)
		}
		if diff := math.Abs(h.ASCMC[2] - refARMC); diff > tolerance {
			t.Errorf("ARMC = %.7f, want %.7f", h.ASCMC[2], refARMC)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("house calculation failed: %v", err)
	}
}

// TestSiderealDiffersFromTropical checks that sidereal mode takes effect and
// that the difference equals the ayanamsha.
func TestSiderealDiffersFromTropical(t *testing.T) {
	c := newTestCalculator(t)

	err := c.Do(context.Background(), func(s *Session) error {
		trop, err := s.Calc(refJD, libswe.Sun, Options{})
		if err != nil {
			return err
		}
		sidOpts := Options{Sidereal: true, Ayanamsha: libswe.SidmLahiri}
		sid, err := s.Calc(refJD, libswe.Sun, sidOpts)
		if err != nil {
			return err
		}
		ayan, err := s.Ayanamsha(refJD, sidOpts)
		if err != nil {
			return err
		}
		// The Lahiri ayanamsha is about 23.7 degrees in 1990.
		if ayan < 23 || ayan > 24.5 {
			t.Errorf("Lahiri ayanamsha = %.4f, want between 23 and 24.5", ayan)
		}
		got := math.Mod(trop.Longitude-sid.Longitude+360, 360)
		if diff := math.Abs(got - ayan); diff > 1e-6 {
			t.Errorf("tropical minus sidereal = %.7f, ayanamsha is %.7f", got, ayan)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("sidereal calculation failed: %v", err)
	}
}

// TestOptionsDoNotLeakBetweenCalls checks that reusing a worker thread does not
// carry settings over. A sidereal and a topocentric call run first, then a
// plain tropical call must still return the reference value.
func TestOptionsDoNotLeakBetweenCalls(t *testing.T) {
	c := newTestCalculator(t)

	err := c.Do(context.Background(), func(s *Session) error {
		if _, err := s.Calc(refJD, libswe.Sun, Options{
			Sidereal: true, Ayanamsha: libswe.SidmKrishnamurti,
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
			t.Errorf("settings leaked from an earlier call: Sun = %.7f, want %.7f",
				res.Longitude, refPlanets[libswe.Sun])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("calculation failed: %v", err)
	}
}

// TestConcurrentCalls checks that concurrent requests return the same result.
// Run with -race it also shows there is no data race.
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
					t.Errorf("concurrent calculation drifted: %.7f", res.Longitude)
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
		t.Errorf("concurrent calculation failed: %v", err)
	}
}

// TestSessionEscapePanics checks that using a Session outside Do fails loudly
// instead of quietly computing on the wrong thread.
func TestSessionEscapePanics(t *testing.T) {
	c := newTestCalculator(t)

	var escaped *Session
	if err := c.Do(context.Background(), func(s *Session) error {
		escaped = s
		return nil
	}); err != nil {
		t.Fatalf("Do failed: %v", err)
	}

	defer func() {
		if recover() == nil {
			t.Error("using an escaped Session should panic")
		}
	}()
	_, _ = escaped.Calc(refJD, libswe.Sun, Options{})
}

func TestMissingEphePathFails(t *testing.T) {
	if _, err := New(Config{EphePath: "../../ephe-does-not-exist"}); err == nil {
		t.Error("a missing ephemeris directory should be an error")
	}
}

func TestUTCToJD(t *testing.T) {
	_, ut, err := UTCToJD(1990, 6, 15, 14, 30, 0)
	if err != nil {
		t.Fatalf("UTCToJD failed: %v", err)
	}
	// swe_utc_to_jd accounts for leap seconds, so the result differs from a
	// plain swe_julday value by a few seconds.
	if diff := math.Abs(ut - refJD); diff > 1e-4 {
		t.Errorf("UT = %.9f, reference %.9f (off by %.2e days)", ut, refJD, diff)
	}
}

// BenchmarkNatalChart measures a full natal chart: the calls needed for ten
// bodies plus the twelve house cusps.
func BenchmarkNatalChart(b *testing.B) {
	c, err := New(Config{EphePath: "../../ephe"})
	if err != nil {
		b.Fatalf("could not start calculator: %v", err)
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
			b.Fatalf("calculation failed: %v", err)
		}
	}
}

// BenchmarkNatalChartParallel measures the same work under concurrent load.
func BenchmarkNatalChartParallel(b *testing.B) {
	c, err := New(Config{EphePath: "../../ephe"})
	if err != nil {
		b.Fatalf("could not start calculator: %v", err)
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
				b.Fatalf("calculation failed: %v", err)
			}
		}
	})
}
