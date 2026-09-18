package version

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The VERSION file is what a compose build stamps into the binary, so it has to
// exist and be a plausible version.
func TestVersionFileIsPresentAndWellFormed(t *testing.T) {
	// The test runs from backend/version, and the file lives at the repo root.
	data, err := os.ReadFile(filepath.Join("..", "..", "VERSION"))
	if err != nil {
		t.Fatalf("the VERSION file is missing, so compose builds would report %q: %v", Version, err)
	}

	value := strings.TrimSpace(string(data))
	if !strings.HasPrefix(value, "v") {
		t.Errorf("VERSION holds %q, want something like v2.4.0", value)
	}
	if strings.Count(value, ".") < 2 {
		t.Errorf("VERSION holds %q, want a major.minor.patch version", value)
	}
	if strings.ContainsAny(value, " \t\n") {
		t.Errorf("VERSION holds %q, which has whitespace in it", value)
	}
}

func TestCurrentReportsTheBuild(t *testing.T) {
	originalVersion, originalCommit := Version, Commit
	t.Cleanup(func() { Version, Commit = originalVersion, originalCommit })

	Version, Commit = "v9.9.9", "abc1234"
	info := Current()
	if info.Version != "v9.9.9" || info.Commit != "abc1234" {
		t.Errorf("Current() = %+v", info)
	}

	// A commit is optional and must simply be omitted when unknown.
	Commit = ""
	if info := Current(); info.Commit != "" {
		t.Errorf("Current() = %+v, want no commit", info)
	}
}
