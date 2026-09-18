package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wzin/realmrunner/backup"
	"github.com/wzin/realmrunner/minecraft"
)

func newUpgradeManager(t *testing.T) *Manager {
	t.Helper()

	m := newWideManager(t)
	if err := backup.InitBackupTable(m.GetDB()); err != nil {
		t.Fatalf("InitBackupTable: %v", err)
	}
	return m
}

// An upgrade to a version the image cannot run must be refused before anything
// is touched, rather than leaving a server that dies on start.
func TestUpgradeRefusesUnrunnableVersion(t *testing.T) {
	m := newUpgradeManager(t)

	root := t.TempDir()
	t.Cleanup(minecraft.SetJavaSearchDirs(root))
	fakeJavaRuntime(t, root, "21")

	srv, err := m.CreateServer("realm", "1.21.11", "vanilla", freeTestPort(t))
	if err != nil {
		t.Fatal(err)
	}
	writeJar(t, m, srv.ID, "old-jar")

	_, err = m.UpgradeServer(srv.ID, "26.3", "vanilla")
	if err == nil {
		t.Fatal("expected the upgrade to be refused")
	}
	if !strings.Contains(err.Error(), "Java 25") {
		t.Errorf("error %q does not explain the Java requirement", err)
	}

	// Nothing may have changed.
	got, _ := m.GetServer(srv.ID)
	if got.Version != "1.21.11" {
		t.Errorf("version = %q, want the original", got.Version)
	}
	if data, _ := os.ReadFile(jarPathFor(m, srv.ID)); string(data) != "old-jar" {
		t.Errorf("the jar was replaced: %q", data)
	}
}

// A download failure must restore the previous jar and version, and leave the
// backup it took in place.
func TestUpgradeRollsBackOnDownloadFailure(t *testing.T) {
	m := newUpgradeManager(t)

	srv, err := m.CreateServer("realm", "26.3", "vanilla", freeTestPort(t))
	if err != nil {
		t.Fatal(err)
	}
	writeJar(t, m, srv.ID, "old-jar")

	// A version that does not exist upstream fails to download.
	result, err := m.UpgradeServer(srv.ID, "99.9", "vanilla")
	if err == nil {
		t.Fatal("expected the upgrade to fail")
	}
	if result == nil || !result.RolledBack {
		t.Fatalf("result = %+v, want a rolled back upgrade", result)
	}
	if result.BackupID == "" {
		t.Error("no backup was taken before the upgrade")
	}

	got, _ := m.GetServer(srv.ID)
	if got.Version != "26.3" {
		t.Errorf("version = %q, want 26.3 restored", got.Version)
	}
	if data, _ := os.ReadFile(jarPathFor(m, srv.ID)); string(data) != "old-jar" {
		t.Errorf("the previous jar was not restored: %q", data)
	}

	backups, err := backup.ListBackups(m.GetDB(), srv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) == 0 {
		t.Error("the pre-upgrade backup is gone")
	}
}

// A sleeping server can be upgraded: it is not running, its port is only held.
func TestUpgradeAllowedWhileSleeping(t *testing.T) {
	m := newUpgradeManager(t)

	srv, err := m.CreateServer("realm", "26.3", "vanilla", freeTestPort(t))
	if err != nil {
		t.Fatal(err)
	}
	writeJar(t, m, srv.ID, "old-jar")
	if err := UpdateServerStatus(m.GetDB(), srv.ID, StatusSleeping); err != nil {
		t.Fatal(err)
	}

	// The upgrade still fails (the version does not exist), but it must fail
	// while downloading rather than be refused outright.
	_, err = m.UpgradeServer(srv.ID, "99.9", "vanilla")
	if err == nil {
		t.Fatal("expected a download failure")
	}
	if strings.Contains(err.Error(), "must be stopped") {
		t.Errorf("a sleeping server was refused: %v", err)
	}
}

func TestUpgradeRefusedWhileRunning(t *testing.T) {
	m := newUpgradeManager(t)

	srv, err := m.CreateServer("realm", "26.3", "vanilla", freeTestPort(t))
	if err != nil {
		t.Fatal(err)
	}
	UpdateServerStatus(m.GetDB(), srv.ID, StatusRunning)

	if _, err := m.UpgradeServer(srv.ID, "1.21.11", "vanilla"); err == nil {
		t.Error("expected the upgrade to be refused for a running server")
	}
}

func jarPathFor(m *Manager, id string) string {
	return filepath.Join(m.GetServerDir(id), "server.jar")
}

func writeJar(t *testing.T, m *Manager, id, content string) {
	t.Helper()

	if err := os.WriteFile(jarPathFor(m, id), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
