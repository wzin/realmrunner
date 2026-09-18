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
	path, _, _ := ResolveJava(requiredMajor)
	return path
}

// ResolveJava reports which runtime satisfies a requirement. major is the Java
// major version of the chosen runtime (0 when falling back to PATH, where the
// version is unknown), and satisfied is false when every installed runtime is
// too old - the case that makes a server exit instantly with
// UnsupportedClassVersionError.
func ResolveJava(requiredMajor int) (path string, major int, satisfied bool) {
	if override := os.Getenv(fmt.Sprintf("REALMRUNNER_JAVA_%d", requiredMajor)); override != "" {
		return override, requiredMajor, true
	}

	installed := installedJavaRuntimes()
	if path, ok := installed[requiredMajor]; ok {
		return path, requiredMajor, true
	}

	majors := make([]int, 0, len(installed))
	for m := range installed {
		majors = append(majors, m)
	}
	sort.Ints(majors)
	for _, m := range majors {
		if m >= requiredMajor {
			return installed[m], m, true
		}
	}

	// Nothing installed is new enough. Fall back to PATH, but say so: with no
	// runtimes discovered at all we cannot tell what "java" is.
	if len(majors) == 0 {
		return "java", 0, true
	}
	return installed[majors[len(majors)-1]], majors[len(majors)-1], false
}

// InstalledJavaMajors lists the Java major versions available to this process.
func InstalledJavaMajors() []int {
	installed := installedJavaRuntimes()
	majors := make([]int, 0, len(installed))
	for m := range installed {
		majors = append(majors, m)
	}
	sort.Ints(majors)
	return majors
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

// javaArgs builds the JVM arguments used by every flavor: the heap size plus
// the G1 tuning Minecraft server operators have converged on (Aikar's flags).
//
// The defaults give long stop-the-world pauses on a server with players on it,
// which show up as "Can't keep up!" in the log and as timeouts for players,
// because the server misses their keep-alives while it is paused.
func javaArgs(memoryMB int) []string {
	args := []string{
		fmt.Sprintf("-Xmx%dM", memoryMB),
		fmt.Sprintf("-Xms%dM", memoryMB),
	}
	args = append(args, GCFlags(memoryMB)...)
	return append(args, "-jar", "server.jar", "nogui")
}

// GCFlags returns G1 settings tuned for a Minecraft server of the given heap
// size. The two values that change with heap size are the new-generation sizes
// and the region size, following Aikar's published flags.
func GCFlags(memoryMB int) []string {
	newSizePercent, maxNewSizePercent, heapRegionSize, reservePercent := "30", "40", "8M", "20"
	if memoryMB >= 12*1024 {
		newSizePercent, maxNewSizePercent, heapRegionSize, reservePercent = "40", "50", "16M", "15"
	}

	return []string{
		"-XX:+UseG1GC",
		"-XX:+ParallelRefProcEnabled",
		"-XX:MaxGCPauseMillis=200",
		"-XX:+UnlockExperimentalVMOptions",
		"-XX:+DisableExplicitGC",
		"-XX:+AlwaysPreTouch",
		"-XX:G1NewSizePercent=" + newSizePercent,
		"-XX:G1MaxNewSizePercent=" + maxNewSizePercent,
		"-XX:G1HeapRegionSize=" + heapRegionSize,
		"-XX:G1ReservePercent=" + reservePercent,
		"-XX:G1HeapWastePercent=5",
		"-XX:G1MixedGCCountTarget=4",
		"-XX:InitiatingHeapOccupancyPercent=15",
		"-XX:G1MixedGCLiveThresholdPercent=90",
		"-XX:G1RSetUpdatingPauseTimePercent=5",
		"-XX:SurvivorRatio=32",
		"-XX:+PerfDisableSharedMem",
		"-XX:MaxTenuringThreshold=1",

		// Record what the heap and the pauses actually look like. Resident
		// memory says nothing about heap pressure, because the heap is
		// committed up front; the size after a collection is the live set.
		"-Xlog:gc:file=" + GCLogName + "::filecount=3,filesize=8M",
	}
}

// GCLogName is where the JVM records collections, relative to the server
// directory.
const GCLogName = "logs/gc.log"
