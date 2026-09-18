package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wzin/realmrunner/minecraft"
)

func writeGCLog(t *testing.T, serverDir, content string) {
	t.Helper()

	path := filepath.Join(serverDir, minecraft.GCLogName)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// A heap that is still nearly full right after a collection is the signature of
// a server that needs more memory - which resident memory cannot show, because
// the heap is committed up front.
func TestMemoryReportDetectsHeapPressure(t *testing.T) {
	dir := t.TempDir()
	writeGCLog(t, dir, `[2026-09-18T10:00:01.000+0000] GC(1) Pause Young (Normal) (G1 Evacuation Pause) 1800M->1700M(2048M) 120.500ms
[2026-09-18T10:00:05.000+0000] GC(2) Pause Young (Normal) (G1 Evacuation Pause) 1900M->1780M(2048M) 210.250ms
[2026-09-18T10:00:09.000+0000] GC(3) Pause Full (G1 Compaction Pause) 2040M->1810M(2048M) 2400.750ms
`)

	report, err := ReadMemoryReport(dir, 2048)
	if err != nil {
		t.Fatalf("ReadMemoryReport: %v", err)
	}

	if report.Collections != 3 {
		t.Errorf("collections = %d, want 3", report.Collections)
	}
	if report.FullGCs != 1 {
		t.Errorf("full GCs = %d, want 1", report.FullGCs)
	}
	if report.LiveSetMB != 1810 {
		t.Errorf("live set = %dMB, want 1810", report.LiveSetMB)
	}
	if report.PeakUsedMB != 2040 {
		t.Errorf("peak used = %dMB, want 2040", report.PeakUsedMB)
	}
	if report.UsedPercent < 85 {
		t.Errorf("used = %d%%, want it recognised as high", report.UsedPercent)
	}
	if report.LongestPause != 2400 {
		t.Errorf("longest pause = %dms, want 2400", report.LongestPause)
	}
	if !strings.Contains(report.Verdict, "Short of heap") {
		t.Errorf("verdict = %q", report.Verdict)
	}
	if !strings.Contains(report.Recommendation, "4096") {
		t.Errorf("recommendation = %q, want about double the live set", report.Recommendation)
	}
	if !strings.Contains(report.Verdict, "2400ms") {
		t.Errorf("a pause that long should be called out: %q", report.Verdict)
	}
}

func TestMemoryReportComfortableHeap(t *testing.T) {
	dir := t.TempDir()
	writeGCLog(t, dir, `[2026-09-18T10:00:01.000+0000] GC(1) Pause Young (Normal) (G1 Evacuation Pause) 900M->300M(4096M) 18.500ms
[2026-09-18T10:02:01.000+0000] GC(2) Pause Young (Normal) (G1 Evacuation Pause) 950M->320M(4096M) 21.000ms
`)

	report, err := ReadMemoryReport(dir, 4096)
	if err != nil {
		t.Fatal(err)
	}

	if report.FullGCs != 0 {
		t.Errorf("full GCs = %d, want 0", report.FullGCs)
	}
	if report.UsedPercent > 20 {
		t.Errorf("used = %d%%, want it recognised as low", report.UsedPercent)
	}
	if !strings.Contains(report.Verdict, "Comfortable") {
		t.Errorf("verdict = %q", report.Verdict)
	}
	if report.Recommendation != "" {
		t.Errorf("no recommendation expected, got %q", report.Recommendation)
	}
}

func TestMemoryReportTightButWorking(t *testing.T) {
	dir := t.TempDir()
	writeGCLog(t, dir, `[2026-09-18T10:00:01.000+0000] GC(1) Pause Young (Normal) (G1 Evacuation Pause) 1800M->1400M(2048M) 60.000ms
`)

	report, err := ReadMemoryReport(dir, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report.Verdict, "little room to spare") {
		t.Errorf("verdict = %q", report.Verdict)
	}
	// A live set of 1400MB suggests roughly double, rounded up to a whole GB.
	if !strings.Contains(report.Recommendation, "3072") {
		t.Errorf("recommendation = %q", report.Recommendation)
	}
}

// The heap size is taken from the log when the caller does not know it.
func TestMemoryReportInfersHeapFromLog(t *testing.T) {
	dir := t.TempDir()
	writeGCLog(t, dir, "[2026-09-18T10:00:01.000+0000] GC(1) Pause Young (Normal) 900M->300M(8192M) 18.500ms\n")

	report, err := ReadMemoryReport(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if report.HeapMB != 8192 {
		t.Errorf("heap = %dMB, want 8192 from the log", report.HeapMB)
	}
}

func TestMemoryReportWithoutCollections(t *testing.T) {
	dir := t.TempDir()
	writeGCLog(t, dir, "[2026-09-18T10:00:00.000+0000] Using G1\n")

	report, err := ReadMemoryReport(dir, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if report.Collections != 0 || !strings.Contains(report.Verdict, "nothing to judge") {
		t.Errorf("report = %+v", report)
	}
}

func TestMemoryReportMissingLog(t *testing.T) {
	if _, err := ReadMemoryReport(t.TempDir(), 2048); err == nil {
		t.Error("expected an error when the server has no GC log")
	}
}

func TestRoundUpToGB(t *testing.T) {
	cases := map[int]int{100: 1024, 1024: 1024, 1500: 2048, 3620: 4096, 4096: 4096}
	for input, want := range cases {
		if got := roundUpToGB(input); got != want {
			t.Errorf("roundUpToGB(%d) = %d, want %d", input, got, want)
		}
	}
}

// Setting a container limit equal to the heap gets the server killed by the
// kernel instead of reporting an out-of-memory error, so it is refused.
func TestSetHeapRequiresContainerHeadroom(t *testing.T) {
	m := newWideManager(t)

	srv, err := m.CreateServer("realm", "26.3", "vanilla", freeTestPort(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := UpdateServerLimits(m.GetDB(), srv.ID, 0, 4096); err != nil {
		t.Fatal(err)
	}

	if err := m.SetHeapMB(srv.ID, 4096); err == nil {
		t.Error("a heap equal to the container limit should be refused")
	}

	// With headroom it is accepted.
	if err := m.SetHeapMB(srv.ID, 2048); err != nil {
		t.Errorf("a 2048MB heap under a 4096MB limit was refused: %v", err)
	}

	got, _ := m.GetServer(srv.ID)
	if got.HeapMB != 2048 {
		t.Errorf("heap = %d, want 2048", got.HeapMB)
	}
}

func TestRequiredContainerMB(t *testing.T) {
	// A 2GB heap needs roughly 3GB of container memory.
	if got := RequiredContainerMB(2048); got < 2816 || got > 3072 {
		t.Errorf("RequiredContainerMB(2048) = %d, want roughly 3000", got)
	}
	if RequiredContainerMB(4096) <= RequiredContainerMB(2048) {
		t.Error("the requirement must grow with the heap")
	}
}

func TestSetHeapRejectsTinyHeap(t *testing.T) {
	m := newWideManager(t)

	srv, err := m.CreateServer("realm", "26.3", "vanilla", freeTestPort(t))
	if err != nil {
		t.Fatal(err)
	}

	if err := m.SetHeapMB(srv.ID, 128); err == nil {
		t.Error("a 128MB heap should be refused")
	}
	// Zero means "use the instance default" and is always allowed.
	if err := m.SetHeapMB(srv.ID, 0); err != nil {
		t.Errorf("clearing the heap setting failed: %v", err)
	}
}
