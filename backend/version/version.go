// Package version carries the build identity of this RealmRunner binary.
package version

// Version is the release this binary was built from. The Docker build sets it
// with -ldflags; a local build reports "dev".
var Version = "dev"

// Commit is the git revision this binary was built from, when known.
var Commit = ""

// Info describes the running build.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit,omitempty"`
}

// Current returns what this binary was built from.
func Current() Info {
	return Info{Version: Version, Commit: Commit}
}
