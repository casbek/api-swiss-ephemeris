// Package version reports which build of the service is running.
//
// This is not only diagnostics. The AGPL requires that users of a network
// service be able to obtain the source of the version they are talking to, so
// the running build has to be identifiable.
package version

import (
	"runtime/debug"
	"sync"
)

// Version is set at build time with
// -ldflags "-X .../internal/version.Version=v1.2.3".
var Version = "dev"

// Info describes the running build.
type Info struct {
	Version  string `json:"version"`
	Revision string `json:"revision,omitempty"`
	Modified bool   `json:"modified,omitempty"`
	BuiltAt  string `json:"built_at,omitempty"`
	Go       string `json:"go"`
}

var (
	once   sync.Once
	cached Info
)

// Get returns the build information, reading it from the embedded build data
// the Go toolchain records.
func Get() Info {
	once.Do(func() {
		cached = Info{Version: Version}

		bi, ok := debug.ReadBuildInfo()
		if !ok {
			return
		}
		cached.Go = bi.GoVersion
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				cached.Revision = s.Value
			case "vcs.time":
				cached.BuiltAt = s.Value
			case "vcs.modified":
				cached.Modified = s.Value == "true"
			}
		}
	})
	return cached
}
