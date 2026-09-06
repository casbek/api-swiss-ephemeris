package swe

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"

	"github.com/casbek/api-swiss-ephemeris/internal/libswe"
)

// ErrPoolClosed is returned when work is submitted to a closed pool.
var ErrPoolClosed = errors.New("swe: pool is closed")

// pool runs Swiss Ephemeris calls on OS threads it owns.
//
// Swiss Ephemeris keeps global state. On Linux with GCC that state is
// thread-local, through the TLS definition in sweodef.h, so every OS thread
// needs its own swe_set_ephe_path call and its own file cache. On Windows the
// thread-local storage is disabled and the state is shared, so concurrent
// calls race.
//
// A single design covers both cases: workers pin themselves to an OS thread
// with runtime.LockOSThread, initialise the library once on that thread, and
// receive work over a channel.
type pool struct {
	tasks chan *task
	quit  chan struct{}
	wg    sync.WaitGroup
	once  sync.Once
}

type task struct {
	fn   func()
	done chan struct{}
}

// newPool starts a pool that reads ephemeris files from ephePath and reserves
// workers OS threads.
//
// One worker is the default and is enough for most deployments: a natal chart
// takes well under a millisecond. On Linux the count can safely be raised
// because the library state is thread-local, though memory grows with it since
// each worker keeps its own ephemeris file cache. On Windows more than one
// worker is not safe.
func newPool(ephePath string, workers int) (*pool, error) {
	if workers < 1 {
		workers = 1
	}
	if runtime.GOOS == "windows" && workers > 1 {
		return nil, fmt.Errorf("swe: Swiss Ephemeris is not thread safe on Windows, workers must be 1, got %d", workers)
	}

	p := &pool{
		tasks: make(chan *task),
		quit:  make(chan struct{}),
	}

	ready := make(chan struct{})
	for i := 0; i < workers; i++ {
		p.wg.Add(1)
		go p.run(ephePath, ready)
	}
	// Wait until every worker has set its ephemeris path.
	for i := 0; i < workers; i++ {
		<-ready
	}
	return p, nil
}

// run is the loop of a single worker. It pins itself to an OS thread,
// initialises Swiss Ephemeris for that thread and then runs tasks in order.
func (p *pool) run(ephePath string, ready chan<- struct{}) {
	defer p.wg.Done()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	libswe.SetEphePath(ephePath)
	// Release the ephemeris files this thread holds open.
	defer libswe.Close()

	ready <- struct{}{}

	for {
		select {
		case t := <-p.tasks:
			t.fn()
			close(t.done)
		case <-p.quit:
			return
		}
	}
}

// do runs fn on a Swiss Ephemeris thread and waits for it to finish.
//
// ctx can only cancel the wait for a free worker. Once a task has been handed
// over it is always awaited, because fn writes to the caller's variables and
// abandoning it would race with the caller reading them.
func (p *pool) do(ctx context.Context, fn func()) error {
	t := &task{fn: fn, done: make(chan struct{})}

	select {
	case p.tasks <- t:
	case <-ctx.Done():
		return ctx.Err()
	case <-p.quit:
		return ErrPoolClosed
	}

	<-t.done
	return nil
}

// close stops the workers and releases the ephemeris files.
func (p *pool) close() {
	p.once.Do(func() {
		close(p.quit)
		p.wg.Wait()
	})
}
