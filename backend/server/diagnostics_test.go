package server

import (
	"strings"
	"testing"
)

// realLogExcerpt is taken from a production log: four players behind one
// address, dropping every few minutes.
var realLogExcerpt = strings.Split(`[09:11:01] [Server thread/INFO]: Ggocio[/88.156.209.37:16008] logged in with entity id 37842 at (-30.4, 102.0, 351.3)
[09:11:01] [Server thread/INFO]: Ggocio joined the game
[09:21:22] [Server thread/INFO]: unbanZEIY[/88.156.209.37:9651] logged in with entity id 40131 at (-36.8, 92.0, 311.6)
[09:48:10] [Server thread/INFO]: unbanZEIY lost connection: You logged in from another location
[09:48:10] [Server thread/INFO]: unbanZEIY left the game
[09:48:10] [Server thread/INFO]: unbanZEIY[/88.156.209.37:42878] logged in with entity id 45590 at (-56.2, 68.0, 489.7)
[09:48:12] [Server thread/INFO]: Ggocio lost connection: You logged in from another location
[09:48:12] [Server thread/INFO]: Ggocio[/88.156.209.37:3144] logged in with entity id 45675 at (-36.4, 92.0, 338.2)
[10:23:37] [Server thread/INFO]: Ggocio lost connection: Disconnected
[10:28:24] [Server thread/INFO]: Ggocio lost connection: Disconnected
[10:33:53] [Server thread/INFO]: Starting minecraft server version 1.21.11
[10:34:20] [Server thread/INFO]: unbanZEIY (/88.156.209.37:30577) lost connection: Disconnected
[10:34:32] [Server thread/INFO]: unbanZEIY (/88.156.209.37:22731) lost connection: Disconnected
[10:34:58] [Server thread/WARN]: Can't keep up! Is the server overloaded? Running 2051ms or 41 ticks behind
[10:37:13] [Server thread/WARN]: Can't keep up! Is the server overloaded? Running 2842ms or 56 ticks behind
[10:41:26] [Server thread/INFO]: Starting minecraft server version 1.21.11
[10:41:40] [Server thread/INFO]: Ggocio[/88.156.209.37:40992] logged in with entity id 586 at (282.1, 40.0, 263.8)
[10:41:43] [Server thread/INFO]: Ggocio (09ce69b6-4022-4d7d-b570-7277244ca40a) lost connection: Timed out
[16:13:42] [Server thread/INFO]: unbanZEIY lost connection: Timed out
[16:14:30] [Server thread/INFO]: JohnMcLane2137[/88.156.209.37:32053] logged in with entity id 15362 at (-77.3, 28.0, 366.6)`, "\n")

func TestAnalyseLogsCountsDisconnectReasons(t *testing.T) {
	diag := AnalyseLogs(realLogExcerpt)

	counts := map[string]int{}
	for _, d := range diag.Disconnects {
		counts[d.Reason] = d.Count
	}

	if counts["Disconnected"] != 4 {
		t.Errorf("Disconnected = %d, want 4", counts["Disconnected"])
	}
	if counts["Timed out"] != 2 {
		t.Errorf("Timed out = %d, want 2", counts["Timed out"])
	}
	if counts["You logged in from another location"] != 2 {
		t.Errorf("duplicate logins = %d, want 2", counts["You logged in from another location"])
	}

	// The most common reason comes first, with an explanation attached.
	if diag.Disconnects[0].Reason != "Disconnected" {
		t.Errorf("first reason = %q", diag.Disconnects[0].Reason)
	}
	if diag.Disconnects[0].Meaning == "" {
		t.Error("the most common reason has no explanation")
	}
}

func TestAnalyseLogsGroupsPlayersAndAddresses(t *testing.T) {
	diag := AnalyseLogs(realLogExcerpt)

	byName := map[string]PlayerSummary{}
	for _, p := range diag.Players {
		byName[p.Name] = p
	}

	if len(byName) != 3 {
		t.Errorf("found players %v, want 3", byName)
	}
	if byName["Ggocio"].Disconnects != 4 {
		t.Errorf("Ggocio disconnects = %d, want 4", byName["Ggocio"].Disconnects)
	}
	if len(byName["Ggocio"].IPs) != 1 || byName["Ggocio"].IPs[0] != "88.156.209.37" {
		t.Errorf("Ggocio addresses = %v", byName["Ggocio"].IPs)
	}

	// The shared address is the thing worth pointing out.
	var sharedNote string
	for _, note := range diag.Notes {
		if strings.Contains(note, "share the address 88.156.209.37") {
			sharedNote = note
		}
	}
	if sharedNote == "" {
		t.Fatalf("no note about the shared address, got %v", diag.Notes)
	}
	for _, name := range []string{"Ggocio", "unbanZEIY", "JohnMcLane2137"} {
		if !strings.Contains(sharedNote, name) {
			t.Errorf("the shared-address note does not mention %s: %s", name, sharedNote)
		}
	}
}

func TestAnalyseLogsCountsLagAndRestarts(t *testing.T) {
	diag := AnalyseLogs(realLogExcerpt)

	if diag.LagWarnings != 2 {
		t.Errorf("lag warnings = %d, want 2", diag.LagWarnings)
	}
	if diag.WorstLagMs != 2842 {
		t.Errorf("worst lag = %dms, want 2842", diag.WorstLagMs)
	}
	if diag.Restarts != 2 {
		t.Errorf("restarts = %d, want 2", diag.Restarts)
	}

	var sawLagNote, sawRestartNote bool
	for _, note := range diag.Notes {
		if strings.Contains(note, "lag warning") {
			sawLagNote = true
		}
		if strings.Contains(note, "started 2 times") {
			sawRestartNote = true
		}
	}
	if !sawLagNote || !sawRestartNote {
		t.Errorf("notes missing lag or restart observation: %v", diag.Notes)
	}
}

// A disconnect during login is logged with the address instead of a bare name;
// it must still be counted.
func TestAnalyseLogsHandlesLoginPhaseDrops(t *testing.T) {
	diag := AnalyseLogs([]string{
		"[10:34:20] [Server thread/INFO]: unbanZEIY (/88.156.209.37:30577) lost connection: Disconnected",
	})

	if len(diag.DisconnectEvents) != 1 {
		t.Fatalf("got %d events, want 1", len(diag.DisconnectEvents))
	}
	event := diag.DisconnectEvents[0]
	if event.Player != "unbanZEIY" || event.IP != "88.156.209.37" || event.Reason != "Disconnected" {
		t.Errorf("event = %+v", event)
	}
}

func TestAnalyseLogsIgnoresOrdinaryLines(t *testing.T) {
	diag := AnalyseLogs([]string{
		"[11:21:50] [Server thread/WARN]: Mismatch in destroy block pos: is{x=-41, y=89, z=287}",
		"[10:49:14] [Server thread/INFO]: [Ggocio: Set the time to 1000]",
		"[11:59:40] [Server thread/INFO]: Server empty for 60 seconds, pausing",
	})

	if len(diag.Disconnects) != 0 || len(diag.Players) != 0 {
		t.Errorf("ordinary lines produced findings: %+v", diag)
	}
}
