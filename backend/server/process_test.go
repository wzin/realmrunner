package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A server writes logs/latest.log itself through log4j. RealmRunner must not
// append to that same file, or the two writers interleave half-lines - which is
// what corrupted the operator's log.
func TestCaptureOutputDoesNotTouchMinecraftLog(t *testing.T) {
	serverDir := t.TempDir()
	logsDir := filepath.Join(serverDir, "logs")
	os.MkdirAll(logsDir, 0755)

	minecraftLog := filepath.Join(logsDir, "latest.log")
	original := "[10:00:00] [Server thread/INFO]: Done (1.0s)! For help, type \"help\"\n"
	if err := os.WriteFile(minecraftLog, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	// A process that writes to both streams.
	process := startEchoProcess(t, serverDir, "out-line", "err-line")
	process.cmd.Wait()
	waitForFile(t, filepath.Join(logsDir, ConsoleLogName))

	after, err := os.ReadFile(minecraftLog)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != original {
		t.Errorf("the Minecraft log was modified:\n%s", after)
	}

	console, err := os.ReadFile(filepath.Join(logsDir, ConsoleLogName))
	if err != nil {
		t.Fatalf("no console log was written: %v", err)
	}
	for _, want := range []string{"out-line", "err-line"} {
		if !strings.Contains(string(console), want) {
			t.Errorf("console log is missing %q:\n%s", want, console)
		}
	}
}

// stderr must be captured while the process runs. Reading it only after stdout
// closed held back JVM errors and stack traces until the server exited.
func TestCaptureOutputRecordsStderrWhileRunning(t *testing.T) {
	serverDir := t.TempDir()

	// This process writes to stderr and then keeps stdout open.
	process := startShellProcess(t, serverDir,
		"echo 'java.lang.OutOfMemoryError: Java heap space' >&2; sleep 3")
	t.Cleanup(func() { process.ForceKill() })

	consolePath := filepath.Join(serverDir, "logs", ConsoleLogName)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(consolePath); err == nil && strings.Contains(string(data), "OutOfMemoryError") {
			return // captured while the process is still running
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Error("stderr was not captured while the process was still running")
}

// The previous run's output is kept so a crash can still be read after the
// automatic restart.
func TestCaptureOutputRotatesPreviousRun(t *testing.T) {
	serverDir := t.TempDir()

	process := startEchoProcess(t, serverDir, "first-run", "")
	process.cmd.Wait()
	waitForFile(t, filepath.Join(serverDir, "logs", ConsoleLogName))

	process = startEchoProcess(t, serverDir, "second-run", "")
	process.cmd.Wait()
	waitForFile(t, filepath.Join(serverDir, "logs", ConsoleLogName))

	previous, err := os.ReadFile(filepath.Join(serverDir, "logs", ConsoleLogName+".1"))
	if err != nil {
		t.Fatalf("the previous run's log was not kept: %v", err)
	}
	if !strings.Contains(string(previous), "first-run") {
		t.Errorf("rotated log holds %q", previous)
	}

	current, _ := os.ReadFile(filepath.Join(serverDir, "logs", ConsoleLogName))
	if !strings.Contains(string(current), "second-run") {
		t.Errorf("current log holds %q", current)
	}
	if strings.Contains(string(current), "first-run") {
		t.Error("the new run appended to the old log instead of rotating it")
	}
}

// Reading logs prefers RealmRunner's own capture, which is a superset of the
// Minecraft log, but still works for servers that only have the old file.
func TestReadHistoricalLogsFallsBackToMinecraftLog(t *testing.T) {
	serverDir := t.TempDir()
	logsDir := filepath.Join(serverDir, "logs")
	os.MkdirAll(logsDir, 0755)
	os.WriteFile(filepath.Join(logsDir, "latest.log"), []byte("old-server-line\n"), 0644)

	lines, err := ReadHistoricalLogs(serverDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0] != "old-server-line" {
		t.Errorf("got %v", lines)
	}

	os.WriteFile(filepath.Join(logsDir, ConsoleLogName), []byte("console-line\n"), 0644)
	lines, err = ReadHistoricalLogs(serverDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0] != "console-line" {
		t.Errorf("got %v, want the console log to win", lines)
	}
}

// startEchoProcess runs a shell that prints to stdout and optionally stderr.
func startEchoProcess(t *testing.T, serverDir, stdout, stderr string) *Process {
	t.Helper()

	script := "echo '" + stdout + "'"
	if stderr != "" {
		script += "; echo '" + stderr + "' >&2"
	}
	return startShellProcess(t, serverDir, script)
}

func startShellProcess(t *testing.T, serverDir, script string) *Process {
	t.Helper()

	// StartProcess insists on a server.jar and writes eula.txt.
	if err := os.WriteFile(filepath.Join(serverDir, "server.jar"), []byte("jar"), 0644); err != nil {
		t.Fatal(err)
	}

	process, err := StartProcess(serverDir, 25565, "sh", []string{"-c", script})
	if err != nil {
		t.Fatalf("StartProcess: %v", err)
	}
	return process
}

func waitForFile(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("%s was never written", path)
}
