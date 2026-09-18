package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wzin/realmrunner/minecraft"
)

// fakeJavaRuntime creates <root>/<major>/bin/java so runtime discovery finds a
// runtime of that version.
func fakeJavaRuntime(t *testing.T, root, major string) {
	t.Helper()

	bin := filepath.Join(root, major, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "java"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestRestartTrackerBacksOffThenGivesUp(t *testing.T) {
	tracker := newRestartTracker()

	var delays []time.Duration
	for i := 0; i < len(restartDelays); i++ {
		delay, ok := tracker.next("srv-1")
		if !ok {
			t.Fatalf("attempt %d was refused", i)
		}
		delays = append(delays, delay)
	}

	for i := 1; i < len(delays); i++ {
		if delays[i] <= delays[i-1] {
			t.Errorf("delay %s did not grow after %s", delays[i], delays[i-1])
		}
	}

	if _, ok := tracker.next("srv-1"); ok {
		t.Error("the tracker kept restarting past its limit")
	}

	// Another server has its own budget.
	if _, ok := tracker.next("srv-2"); !ok {
		t.Error("a different server was refused a restart")
	}

	tracker.reset("srv-1")
	if _, ok := tracker.next("srv-1"); !ok {
		t.Error("reset did not clear the crash streak")
	}
}

func TestCrashReason(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  string
	}{
		{
			name: "too old java runtime",
			lines: []string{
				"Starting server",
				"Error: LinkageError occurred while loading main class net.minecraft.bundler.Main",
				"\tjava.lang.UnsupportedClassVersionError: net/minecraft/bundler/Main has been compiled by a more recent version of the Java Runtime (class file version 69.0), this version of the Java Runtime only recognizes class file versions up to 65.0",
			},
			want: "newer Java runtime",
		},
		{
			name:  "out of memory",
			lines: []string{"[Server thread/ERROR]: java.lang.OutOfMemoryError: Java heap space"},
			want:  "ran out of memory",
		},
		{
			name:  "port in use",
			lines: []string{"[Server thread/WARN]: **** FAILED TO BIND TO PORT!", "Failed to bind to port 25565"},
			want:  "already in use",
		},
		{
			name:  "eula",
			lines: []string{"You need to agree to the EULA in order to run the server."},
			want:  "EULA",
		},
		{
			name:  "nothing recognisable",
			lines: []string{"[Server thread/INFO]: Stopping server"},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := crashReason(tt.lines)
			if tt.want == "" {
				if got != "" {
					t.Errorf("crashReason returned %q, want an empty string", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("crashReason returned %q, want it to mention %q", got, tt.want)
			}
		})
	}
}

// The check that would have turned the 26.x outage into a clear message.
func TestPreflightJava(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(minecraft.SetJavaSearchDirs(root))

	fakeJavaRuntime(t, root, "21")

	if err := preflightJava("1.21.11"); err != nil {
		t.Errorf("preflightJava(1.21.11) with Java 21 installed: %v", err)
	}

	err := preflightJava("26.3")
	if err == nil {
		t.Fatal("preflightJava(26.3) with only Java 21 installed should fail")
	}
	for _, want := range []string{"26.3", "Java 25", "Java 21"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}

	fakeJavaRuntime(t, root, "25")
	if err := preflightJava("26.3"); err != nil {
		t.Errorf("preflightJava(26.3) with Java 25 installed: %v", err)
	}
}
