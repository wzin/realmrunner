package minecraft

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRequiredJavaMajor(t *testing.T) {
	tests := []struct {
		version string
		want    int
	}{
		// Year-based scheme introduced in 2026 needs Java 25
		{"26.1", 25},
		{"26.1.2", 25},
		{"26.3", 25},
		{"26.3-rc-1", 25},
		{"26.3-snapshot-10", 25},
		{"27.0", 25},
		// 1.20.5+ and 1.21.x need Java 21
		{"1.21.11", 21},
		{"1.21", 21},
		{"1.20.6", 21},
		{"1.20.5", 21},
		// 1.17 - 1.20.4 need Java 17
		{"1.20.4", 17},
		{"1.20", 17},
		{"1.18.2", 17},
		{"1.17.1", 17},
		// Legacy versions were built for Java 8
		{"1.16.5", 8},
		{"1.12.2", 8},
		// Weekly snapshots
		{"24w14a", 21},
		{"26w05a", 25},
		// Unparseable input falls back to the previous default
		{"", 21},
		{"garbage", 21},
	}

	for _, tt := range tests {
		if got := RequiredJavaMajor(tt.version); got != tt.want {
			t.Errorf("RequiredJavaMajor(%q) = %d, want %d", tt.version, got, tt.want)
		}
	}
}

// fakeJavaHome creates <root>/<name>/bin/java and returns the binary path.
func fakeJavaHome(t *testing.T, root, name string) string {
	t.Helper()
	bin := filepath.Join(root, name, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	java := filepath.Join(bin, "java")
	if err := os.WriteFile(java, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	return java
}

func TestJavaCommand(t *testing.T) {
	root := t.TempDir()
	java21 := fakeJavaHome(t, root, "21")
	java25 := fakeJavaHome(t, root, "25")
	// A directory without a java binary must be ignored
	if err := os.MkdirAll(filepath.Join(root, "17"), 0755); err != nil {
		t.Fatal(err)
	}

	original := javaSearchDirs
	javaSearchDirs = []string{root}
	t.Cleanup(func() { javaSearchDirs = original })

	tests := []struct {
		name     string
		required int
		want     string
	}{
		{"exact match 25", 25, java25},
		{"exact match 21", 21, java21},
		{"older requirement uses oldest compatible runtime", 17, java21},
		{"java 8 requirement falls forward to 21", 8, java21},
		// Nothing installed is new enough: return the newest runtime there is.
		// The caller checks ResolveJava's satisfied flag and refuses to start.
		{"newer than anything installed uses the newest runtime", 99, java25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := JavaCommand(tt.required); got != tt.want {
				t.Errorf("JavaCommand(%d) = %q, want %q", tt.required, got, tt.want)
			}
		})
	}
}

func TestJavaCommandEnvOverride(t *testing.T) {
	root := t.TempDir()
	fakeJavaHome(t, root, "25")

	original := javaSearchDirs
	javaSearchDirs = []string{root}
	t.Cleanup(func() { javaSearchDirs = original })

	t.Setenv("REALMRUNNER_JAVA_25", "/custom/java")
	if got := JavaCommand(25); got != "/custom/java" {
		t.Errorf("JavaCommand(25) = %q, want /custom/java", got)
	}
}

func TestJavaCommandForVersion(t *testing.T) {
	root := t.TempDir()
	java21 := fakeJavaHome(t, root, "21")
	java25 := fakeJavaHome(t, root, "25")

	original := javaSearchDirs
	javaSearchDirs = []string{root}
	t.Cleanup(func() { javaSearchDirs = original })

	if got := JavaCommandForVersion("26.3"); got != java25 {
		t.Errorf("26.3 resolved to %q, want %q", got, java25)
	}
	if got := JavaCommandForVersion("1.21.4"); got != java21 {
		t.Errorf("1.21.4 resolved to %q, want %q", got, java21)
	}
}

func TestJavaCommandNoInstallations(t *testing.T) {
	original := javaSearchDirs
	javaSearchDirs = []string{filepath.Join(t.TempDir(), "missing")}
	t.Cleanup(func() { javaSearchDirs = original })

	if got := JavaCommand(25); got != "java" {
		t.Errorf("JavaCommand(25) = %q, want java", got)
	}
}

func TestJavaArgs(t *testing.T) {
	args := javaArgs(2048)
	want := []string{"-Xmx2048M", "-Xms2048M", "-jar", "server.jar", "nogui"}
	if len(args) != len(want) {
		t.Fatalf("javaArgs returned %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("javaArgs()[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

func TestResolveJavaReportsUnsatisfiableRequirements(t *testing.T) {
	root := t.TempDir()
	java21 := fakeJavaHome(t, root, "21")

	original := javaSearchDirs
	javaSearchDirs = []string{root}
	t.Cleanup(func() { javaSearchDirs = original })

	path, major, satisfied := ResolveJava(21)
	if path != java21 || major != 21 || !satisfied {
		t.Errorf("ResolveJava(21) = %q, %d, %v; want %q, 21, true", path, major, satisfied, java21)
	}

	// Minecraft 26.x on a Java 21-only image: this is the failure that used to
	// show up only as an instant, unexplained shutdown.
	path, major, satisfied = ResolveJava(25)
	if satisfied {
		t.Error("ResolveJava(25) reported satisfied with only Java 21 installed")
	}
	if path != java21 || major != 21 {
		t.Errorf("ResolveJava(25) = %q, %d; want the newest installed runtime %q, 21", path, major, java21)
	}
}

func TestInstalledJavaMajors(t *testing.T) {
	root := t.TempDir()
	fakeJavaHome(t, root, "21")
	fakeJavaHome(t, root, "25")

	original := javaSearchDirs
	javaSearchDirs = []string{root}
	t.Cleanup(func() { javaSearchDirs = original })

	majors := InstalledJavaMajors()
	if len(majors) != 2 || majors[0] != 21 || majors[1] != 25 {
		t.Errorf("InstalledJavaMajors() = %v, want [21 25]", majors)
	}
}
