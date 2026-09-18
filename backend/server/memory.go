package server

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/wzin/realmrunner/minecraft"
)

// MemoryReport says whether a server is short of heap. Resident memory cannot
// answer that: RealmRunner starts the JVM with -Xms equal to -Xmx, so the whole
// heap is committed immediately and the process looks the same size whether it
// is busy or idle. What matters is how much survives a collection.
type MemoryReport struct {
	HeapMB int `json:"heap_mb"`
	// LiveSetMB is the largest heap occupancy measured right after a
	// collection: the memory the world actually needs.
	LiveSetMB int `json:"live_set_mb"`
	// PeakUsedMB is the largest occupancy seen before a collection.
	PeakUsedMB int `json:"peak_used_mb"`
	// UsedPercent is the live set as a share of the heap.
	UsedPercent    int    `json:"used_percent"`
	Collections    int    `json:"collections"`
	FullGCs        int    `json:"full_gcs"`
	LongestPause   int    `json:"longest_pause_ms"`
	TotalPauseMs   int    `json:"total_pause_ms"`
	Verdict        string `json:"verdict"`
	Recommendation string `json:"recommendation,omitempty"`
}

// gcPauseRe matches the occupancy and pause of one collection, e.g.
// "1024M->512M(2048M) 15.123ms".
var gcPauseRe = regexp.MustCompile(`(\d+)M->(\d+)M\((\d+)M\)\s+([\d.]+)ms`)

// ReadMemoryReport parses a server's GC log.
func ReadMemoryReport(serverDir string, heapMB int) (*MemoryReport, error) {
	file, err := os.Open(filepath.Join(serverDir, minecraft.GCLogName))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	report := &MemoryReport{HeapMB: heapMB}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLogLine)
	for scanner.Scan() {
		line := scanner.Text()

		match := gcPauseRe.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		before, _ := strconv.Atoi(match[1])
		after, _ := strconv.Atoi(match[2])
		total, _ := strconv.Atoi(match[3])
		pause, _ := strconv.ParseFloat(match[4], 64)

		report.Collections++
		if isFullGC(line) {
			report.FullGCs++
		}
		if after > report.LiveSetMB {
			report.LiveSetMB = after
		}
		if before > report.PeakUsedMB {
			report.PeakUsedMB = before
		}
		if report.HeapMB == 0 && total > 0 {
			report.HeapMB = total
		}
		if ms := int(pause); ms > report.LongestPause {
			report.LongestPause = ms
		}
		report.TotalPauseMs += int(pause)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	report.summarise()
	return report, nil
}

func isFullGC(line string) bool {
	for _, marker := range []string{"Pause Full", "Full GC"} {
		if containsFold(line, marker) {
			return true
		}
	}
	return false
}

func containsFold(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) && indexFold(haystack, needle) >= 0
}

func indexFold(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// summarise turns the numbers into a verdict an operator can act on.
func (r *MemoryReport) summarise() {
	if r.Collections == 0 {
		r.Verdict = "No collections recorded yet, so there is nothing to judge."
		return
	}
	if r.HeapMB > 0 {
		r.UsedPercent = r.LiveSetMB * 100 / r.HeapMB
	}

	switch {
	case r.UsedPercent >= 85 || r.FullGCs > 0:
		r.Verdict = "Short of heap. The world still needs most of the heap right after a collection, so the JVM is collecting constantly and pausing the server while it does."
		r.Recommendation = "Raise the heap to about " + itoa(roundUpToGB(r.LiveSetMB*2)) + " MB."
	case r.UsedPercent >= 65:
		r.Verdict = "Working, with little room to spare. A big explosion or a few players exploring at once would push it into constant collections."
		r.Recommendation = "Consider raising the heap to about " + itoa(roundUpToGB(r.LiveSetMB*2)) + " MB."
	default:
		r.Verdict = "Comfortable. The heap is larger than what the world needs."
	}

	if r.LongestPause >= 1000 {
		r.Verdict += " The longest pause was " + itoa(r.LongestPause) + "ms, long enough for players to feel it."
	}
}

// roundUpToGB rounds a megabyte figure up to the next whole gigabyte, with a
// 1 GB floor.
func roundUpToGB(mb int) int {
	if mb < 1024 {
		return 1024
	}
	if mb%1024 == 0 {
		return mb
	}
	return (mb/1024 + 1) * 1024
}

// MemoryReport reads and analyses one server's GC log.
func (m *Manager) MemoryReport(id string) (*MemoryReport, error) {
	srv, err := GetServer(m.db, id)
	if err != nil {
		return nil, err
	}
	return ReadMemoryReport(m.getServerDir(id), m.heapFor(srv))
}
