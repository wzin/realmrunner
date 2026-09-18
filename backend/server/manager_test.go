package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wzin/realmrunner/config"
	"github.com/wzin/realmrunner/minecraft"
)

func newTestManager(t *testing.T) (*Manager, string) {
	t.Helper()

	dataDir := t.TempDir()
	db, err := InitDB(dataDir)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cfg := &config.Config{
		DataDir:    dataDir,
		MaxRunning: 2,
		MemoryMB:   1024,
		PortRange:  config.PortRange{Min: 25565, Max: 25570},
	}

	return NewManager(db, cfg, nil, minecraft.NewRegistry(), nil), dataDir
}

func TestManagerCreateServer(t *testing.T) {
	m, dataDir := newTestManager(t)

	srv, err := m.CreateServer("survival", "26.3", "", 25565)
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if srv.Flavor != "vanilla" {
		t.Errorf("flavor = %q, want vanilla (the default)", srv.Flavor)
	}

	dir := filepath.Join(dataDir, "servers", srv.ID)
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("server directory was not created: %v", err)
	}
	if m.GetServerDir(srv.ID) != dir {
		t.Errorf("GetServerDir = %q, want %q", m.GetServerDir(srv.ID), dir)
	}
}

func TestManagerCreateServerRejectsBadPorts(t *testing.T) {
	m, _ := newTestManager(t)

	if _, err := m.CreateServer("too low", "26.3", "vanilla", 25000); err == nil {
		t.Error("a port below the configured range should be rejected")
	}
	if _, err := m.CreateServer("too high", "26.3", "vanilla", 26000); err == nil {
		t.Error("a port above the configured range should be rejected")
	}

	if _, err := m.CreateServer("first", "26.3", "vanilla", 25566); err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if _, err := m.CreateServer("duplicate", "26.3", "vanilla", 25566); err == nil {
		t.Error("a duplicate port should be rejected")
	}
}

func TestManagerStartServerRespectsMaxRunning(t *testing.T) {
	m, _ := newTestManager(t)

	for i, port := range []int{25565, 25566, 25567} {
		srv, err := m.CreateServer("srv", "26.3", "vanilla", port)
		if err != nil {
			t.Fatalf("CreateServer: %v", err)
		}
		// Simulate the first two already running, up to MaxRunning.
		if i < 2 {
			if err := UpdateServerStatus(m.GetDB(), srv.ID, StatusRunning); err != nil {
				t.Fatal(err)
			}
			continue
		}

		err = m.StartServer(srv.ID)
		if err == nil {
			t.Fatal("starting a third server should hit the max running limit")
		}
	}
}

// A server whose jar was never downloaded must fail to start with a clear
// error and be left in the stopped state, not "starting".
func TestManagerStartServerWithoutJar(t *testing.T) {
	m, _ := newTestManager(t)

	srv, err := m.CreateServer("no jar", "26.3", "vanilla", 25565)
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}

	if err := m.StartServer(srv.ID); err == nil {
		t.Fatal("expected an error when server.jar is missing")
	}

	got, err := m.GetServer(srv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusStopped {
		t.Errorf("status = %q, want %q", got.Status, StatusStopped)
	}
}

func TestManagerSetLimitsAndSchedule(t *testing.T) {
	m, _ := newTestManager(t)

	srv, err := m.CreateServer("limits", "1.21.11", "paper", 25565)
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}

	if err := m.SetLimits(srv.ID, 2.0, 3072); err != nil {
		t.Fatalf("SetLimits: %v", err)
	}
	if err := m.SetRestartSchedule(srv.ID, "0 5 * * *"); err != nil {
		t.Fatalf("SetRestartSchedule: %v", err)
	}

	got, err := m.GetServer(srv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CPULimit != 2.0 || got.MemoryLimitMB != 3072 || got.RestartSchedule != "0 5 * * *" {
		t.Errorf("unexpected server state: %+v", got)
	}
}

func TestManagerWipeoutServer(t *testing.T) {
	m, dataDir := newTestManager(t)

	srv, err := m.CreateServer("doomed", "26.3", "vanilla", 25565)
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	worldFile := filepath.Join(dataDir, "servers", srv.ID, "world.dat")
	if err := os.WriteFile(worldFile, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := m.WipeoutServer(srv.ID); err != nil {
		t.Fatalf("WipeoutServer: %v", err)
	}

	if _, err := m.GetServer(srv.ID); err == nil {
		t.Error("server record still present after wipeout")
	}
	if _, err := os.Stat(filepath.Join(dataDir, "servers", srv.ID)); !os.IsNotExist(err) {
		t.Error("server directory still present after wipeout")
	}
}
