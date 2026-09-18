package server

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wzin/realmrunner/minecraft"
)

// exitDescriptions carries the description of how a process ended from the
// monitor goroutine, which has the Wait error, to the crash handler.
type exitDescriptions struct {
	mu   sync.Mutex
	last map[string]string
}

func newExitDescriptions() *exitDescriptions {
	return &exitDescriptions{last: make(map[string]string)}
}

func (e *exitDescriptions) set(id, description string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.last[id] = description
}

// take returns the stored description, falling back to the exit code.
func (e *exitDescriptions) take(id string, exitCode int) string {
	e.mu.Lock()
	defer e.mu.Unlock()

	description, ok := e.last[id]
	delete(e.last, id)
	if !ok || description == "" {
		return fmt.Sprintf("The server process exited unexpectedly (exit code %d).", exitCode)
	}
	return description
}

// restartDelays is the backoff applied to successive crash restarts. A server
// that keeps dying is left alone rather than restarted forever.
var restartDelays = []time.Duration{
	10 * time.Second,
	30 * time.Second,
	60 * time.Second,
	2 * time.Minute,
	5 * time.Minute,
}

// stableUptime is how long a server must stay up before its crash streak is
// considered over.
const stableUptime = 10 * time.Minute

type restartTracker struct {
	mu       sync.Mutex
	attempts map[string]int
}

func newRestartTracker() *restartTracker {
	return &restartTracker{attempts: make(map[string]int)}
}

// next returns the delay before the next restart attempt, and whether another
// attempt should be made at all.
func (r *restartTracker) next(id string) (time.Duration, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	attempt := r.attempts[id]
	if attempt >= len(restartDelays) {
		return 0, false
	}
	r.attempts[id] = attempt + 1
	return restartDelays[attempt], true
}

func (r *restartTracker) reset(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.attempts, id)
}

func (r *restartTracker) count(id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.attempts[id]
}

// preflightJava checks that a runtime new enough for this Minecraft version is
// installed. Without this check a too-old JVM makes the server exit within a
// second with UnsupportedClassVersionError, which looks like "it just stops".
func preflightJava(version string) error {
	required := minecraft.RequiredJavaMajor(version)
	_, major, satisfied := minecraft.ResolveJava(required)
	if satisfied {
		return nil
	}

	return fmt.Errorf(
		"Minecraft %s requires Java %d but the newest runtime available is Java %d; "+
			"install a Java %d runtime (or set REALMRUNNER_JAVA_%d) before starting this server",
		version, required, major, required, required)
}

// exitCodeOf extracts a process exit code from the error returned by Wait.
func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return -1
}

// exitDescription explains how a process ended. A JVM killed by the kernel (out
// of memory, for instance) reports a signal rather than an exit code.
func exitDescription(err error, exitCode int) string {
	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return fmt.Sprintf("The server process was killed by signal %s, which usually means the host or a memory limit stopped it.", status.Signal())
		}
	}
	return fmt.Sprintf("The server process exited unexpectedly (exit code %d).", exitCode)
}

// crashReason scans the tail of a server log for the line that explains why the
// process died, so the UI can show a cause instead of just "crashed".
func crashReason(logLines []string) string {
	// Look at the tail first: the fatal message is usually the last thing said.
	patterns := []struct {
		needle  string
		explain func(line string) string
	}{
		{"UnsupportedClassVersionError", func(line string) string {
			return "The server jar needs a newer Java runtime than the one it was started with: " + condense(line)
		}},
		{"java.lang.OutOfMemoryError", func(string) string {
			return "The server ran out of memory. Raise the memory limit and try again."
		}},
		{"Failed to bind to port", func(string) string {
			return "The port is already in use by another process."
		}},
		{"You need to agree to the EULA", func(string) string {
			return "The Minecraft EULA was not accepted."
		}},
		{"Exception in thread \"main\"", func(line string) string {
			return "The server failed to start: " + condense(line)
		}},
		{"Cannot read field", func(line string) string {
			return condense(line)
		}},
	}

	for i := len(logLines) - 1; i >= 0; i-- {
		line := logLines[i]
		for _, p := range patterns {
			if strings.Contains(line, p.needle) {
				return p.explain(line)
			}
		}
	}

	return ""
}

func condense(line string) string {
	line = strings.TrimSpace(line)
	if len(line) > 240 {
		line = line[:240] + "..."
	}
	return line
}

// handleCrash records why a server died and, when enabled, schedules a restart
// with backoff.
func (m *Manager) handleCrash(id string, exitCode int) {
	srv, err := GetServer(m.db, id)
	if err != nil {
		log.Printf("Crash handling for %s: %v", id, err)
		return
	}

	reason := ""
	if lines, err := ReadHistoricalLogs(m.getServerDir(id)); err == nil {
		if len(lines) > 200 {
			lines = lines[len(lines)-200:]
		}
		reason = crashReason(lines)
	}
	if reason == "" {
		reason = m.exitDescriptions.take(id, exitCode)
	}

	UpdateServerStatus(m.db, id, StatusCrashed)
	RecordExit(m.db, id, exitCode, reason)
	log.Printf("Server %s crashed (exit code %d): %s", id, exitCode, reason)

	if !srv.AutoRestart {
		return
	}

	delay, ok := m.restarts.next(id)
	if !ok {
		log.Printf("Server %s crashed %d times in a row; not restarting again", id, m.restarts.count(id))
		RecordExit(m.db, id, exitCode, reason+" Automatic restarts gave up after repeated crashes.")
		return
	}

	log.Printf("Restarting server %s in %s (attempt %d)", id, delay, m.restarts.count(id))
	go func() {
		time.Sleep(delay)

		// The operator may have started or removed the server in the meantime.
		current, err := GetServer(m.db, id)
		if err != nil || current.Status != StatusCrashed {
			return
		}
		if err := m.StartServer(id); err != nil {
			log.Printf("Automatic restart of %s failed: %v", id, err)
		}
	}()
}
