package minecraft

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// javaSearchDirs are scanned for side-by-side JRE installations. Each entry is
// expected to contain directories whose name starts with the Java major
// version (e.g. /opt/java/21, /usr/lib/jvm/temurin-25-jre).
var javaSearchDirs = []string{"/opt/java", "/usr/lib/jvm"}

var (
	releaseRe  = regexp.MustCompile(`^(\d+)(?:\.(\d+))?(?:\.(\d+))?`)
	snapshotRe = regexp.MustCompile(`^(\d{2})w\d+[a-z]$`)
)

// RequiredJavaMajor returns the Java major version a Minecraft version needs.
// It is a fallback for when upstream metadata is unavailable; providers that
// can ask upstream (vanilla, Paper) should prefer the reported value.
func RequiredJavaMajor(version string) int {
	v := strings.TrimSpace(version)

	// Year-based snapshots such as 24w14a.
	if m := snapshotRe.FindStringSubmatch(v); m != nil {
		year, _ := strconv.Atoi(m[1])
		if year >= 26 {
			return 25
		}
		return 21
	}

	m := releaseRe.FindStringSubmatch(v)
	if m == nil {
		return 21
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])

	// 2026 onwards Mojang switched to a year-based scheme (26.1, 26.3, ...),
	// which requires Java 25.
	if major >= 26 {
		return 25
	}

	if major == 1 {
		switch {
		case minor >= 21:
			return 21
		case minor == 20 && patch >= 5:
			return 21
		case minor >= 17:
			return 17
		default:
			return 8
		}
	}

	return 21
}

// JavaCommand returns the java binary to use for the given required major
// version. It prefers an exact match, then the oldest installed runtime that is
// new enough, and finally falls back to whatever "java" is on PATH.
func JavaCommand(requiredMajor int) string {
	if override := os.Getenv(fmt.Sprintf("REALMRUNNER_JAVA_%d", requiredMajor)); override != "" {
		return override
	}

	installed := installedJavaRuntimes()
	if path, ok := installed[requiredMajor]; ok {
		return path
	}

	majors := make([]int, 0, len(installed))
	for major := range installed {
		majors = append(majors, major)
	}
	sort.Ints(majors)
	for _, major := range majors {
		if major >= requiredMajor {
			return installed[major]
		}
	}

	return "java"
}

// JavaCommandForVersion resolves the java binary for a Minecraft version.
func JavaCommandForVersion(version string) string {
	return JavaCommand(RequiredJavaMajor(version))
}

// installedJavaRuntimes maps Java major version to a java binary path.
func installedJavaRuntimes() map[int]string {
	runtimes := make(map[int]string)

	for _, dir := range javaSearchDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			major, ok := majorFromDirName(entry.Name())
			if !ok {
				continue
			}
			bin := filepath.Join(dir, entry.Name(), "bin", "java")
			if _, err := os.Stat(bin); err != nil {
				continue
			}
			if _, exists := runtimes[major]; !exists {
				runtimes[major] = bin
			}
		}
	}

	return runtimes
}

var dirMajorRe = regexp.MustCompile(`(\d+)`)

func majorFromDirName(name string) (int, bool) {
	m := dirMajorRe.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	major, err := strconv.Atoi(m[1])
	if err != nil || major <= 0 {
		return 0, false
	}
	return major, true
}

// javaArgs builds the common JVM arguments used by every flavor.
func javaArgs(memoryMB int) []string {
	return []string{
		fmt.Sprintf("-Xmx%dM", memoryMB),
		fmt.Sprintf("-Xms%dM", memoryMB),
		"-jar", "server.jar", "nogui",
	}
}
