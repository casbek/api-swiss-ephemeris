package swe

import (
	"context"
	"math"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/casbek/api-swiss-ephemeris/internal/libswe"
)

// More than one calculation thread is what keeps a long request from holding up
// the short ones behind it. It is also the riskier configuration, since the
// library keeps global state, so these tests check that parallel workers give
// the same answers and that the queue really does drain in parallel.
//
// They cannot run on Windows, where the state is shared between threads rather
// than thread-local and the pool refuses to start more than one. CI runs on
// Linux, which is where the deployed service runs too.

func skipUnlessParallel(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Swiss Ephemeris keeps shared state on Windows, so only one worker is allowed")
	}
}

func TestParallelWorkersAgreeWithOne(t *testing.T) {
	skipUnlessParallel(t)

	single, err := New(Config{EphePath: "../../ephe", Workers: 1})
	if err != nil {
		t.Fatalf("could not start one worker: %v", err)
	}
	defer single.Close()

	many, err := New(Config{EphePath: "../../ephe", Workers: 4})
	if err != nil {
		t.Fatalf("could not start four workers: %v", err)
	}
	defer many.Close()

	read := func(c *Calculator, ipl int) libswe.CalcResult {
		var res libswe.CalcResult
		if err := c.Do(context.Background(), func(s *Session) error {
			var err error
			res, err = s.Calc(refJD, ipl, Options{})
			return err
		}); err != nil {
			t.Fatalf("calculation failed: %v", err)
		}
		return res
	}

	for ipl := range refPlanets {
		a, b := read(single, ipl), read(many, ipl)
		if a.Longitude != b.Longitude || a.SpeedLong != b.SpeedLong {
			t.Errorf("%s: one worker gives %.9f, four give %.9f",
				libswe.PlanetName(ipl), a.Longitude, b.Longitude)
		}
	}
}

// TestParallelWorkersStayCorrectUnderLoad hammers every worker at once. Run
// with -race it also shows that the per-thread state the library keeps is not
// being shared.
func TestParallelWorkersStayCorrectUnderLoad(t *testing.T) {
	skipUnlessParallel(t)

	c, err := New(Config{EphePath: "../../ephe", Workers: 4})
	if err != nil {
		t.Fatalf("could not start four workers: %v", err)
	}
	defer c.Close()

	const goroutines = 64
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			// Half ask in the sidereal zodiac, so the per-request state each
			// worker writes before every calculation is under contention too.
			opts := Options{}
			want := refPlanets[libswe.Sun]
			if i%2 == 0 {
				opts = Options{Sidereal: true, Ayanamsha: libswe.SidmLahiri}
			}

			err := c.Do(context.Background(), func(s *Session) error {
				res, err := s.Calc(refJD, libswe.Sun, opts)
				if err != nil {
					return err
				}
				if opts.Sidereal {
					// The sidereal Sun is the tropical one less the ayanamsha,
					// which is about 23.7 degrees in 1990.
					shift := math.Mod(want-res.Longitude+360, 360)
					if shift < 23 || shift > 24.5 {
						t.Errorf("sidereal Sun is %.6f from tropical, want the ayanamsha", shift)
					}
					return nil
				}
				if math.Abs(res.Longitude-want) > tolerance {
					t.Errorf("Sun = %.7f, want %.7f", res.Longitude, want)
				}
				return nil
			})
			if err != nil {
				t.Errorf("calculation failed: %v", err)
			}
		}(i)
	}
	wg.Wait()
}

// TestParallelWorkersDrainTogether is the reason for the whole arrangement: a
// slow request must not hold up the ones behind it.
func TestParallelWorkersDrainTogether(t *testing.T) {
	skipUnlessParallel(t)

	c, err := New(Config{EphePath: "../../ephe", Workers: 4})
	if err != nil {
		t.Fatalf("could not start four workers: %v", err)
	}
	defer c.Close()

	// Four pieces of work that each hold a thread for a while. With one worker
	// they would run one after another; with four they overlap.
	const hold = 200 * time.Millisecond
	const busy = 4

	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < busy; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.Do(context.Background(), func(s *Session) error {
				time.Sleep(hold)
				return nil
			})
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	// Serialised, this would take four times the hold. Allowing generous
	// slack for scheduling, anything near that means the workers are not
	// running in parallel.
	if elapsed > hold*2 {
		t.Errorf("four concurrent calculations took %v; with %d workers they should "+
			"overlap rather than queue", elapsed, busy)
	}
}
