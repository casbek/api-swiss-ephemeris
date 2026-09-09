package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/casbek/api-swiss-ephemeris/internal/config"
)

// healthcheckTimeout bounds the probe. The health endpoint runs a real
// calculation, which takes microseconds, so anything approaching this means
// the server is wedged rather than busy.
const healthcheckTimeout = 5 * time.Second

// runHealthcheck asks the server running in this container whether it is well,
// and reports the answer through the exit code.
//
// It exists because the image is built on scratch: there is no shell in it and
// no curl, so the only thing that can probe the server is the binary itself.
// Adding a shell to the image for the sake of its health check would undo the
// reason for leaving one out.
func runHealthcheck() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	url := "http://" + probeHost(cfg.Addr) + "/health"

	ctx, cancel := context.WithTimeout(context.Background(), healthcheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach %s: %w", url, err)
	}
	defer resp.Body.Close()

	// The endpoint answers 503 while draining and when the ephemeris files
	// have become unreadable, both of which mean this instance should not be
	// sent work.
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %s", url, resp.Status)
	}
	return nil
}

// probeHost turns a listen address into one that can be dialled from inside
// the same container.
//
// A server listening on 0.0.0.0 is not reachable at that address; the loopback
// is where it actually answers.
func probeHost(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" || strings.EqualFold(host, "[::]") {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

// isHealthcheck reports whether the process was started to probe rather than
// to serve.
func isHealthcheck(args []string) bool {
	for _, a := range args {
		if a == "-healthcheck" || a == "--healthcheck" {
			return true
		}
	}
	return false
}

// exitWith reports a failure to stderr and sets the exit code, for the probe
// path where there is no logger.
func exitWith(err error) {
	fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
	os.Exit(1)
}
