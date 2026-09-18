package server

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := InitDB(t.TempDir())
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newTestServer(t *testing.T, db *sql.DB, id string, port int) *Server {
	t.Helper()

	srv := &Server{
		ID:      id,
		Name:    "test-" + id,
		Version: "26.3",
		Flavor:  "vanilla",
		Port:    port,
		Status:  StatusStopped,
	}
	if err := CreateServer(db, srv); err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	return srv
}

func TestCreateAndGetServer(t *testing.T) {
	db := newTestDB(t)
	newTestServer(t, db, "srv-1", 25565)

	got, err := GetServer(db, "srv-1")
	if err != nil {
		t.Fatalf("GetServer: %v", err)
	}
	if got.Version != "26.3" || got.Flavor != "vanilla" || got.Port != 25565 {
		t.Errorf("unexpected server: %+v", got)
	}
	if got.Status != StatusStopped {
		t.Errorf("status = %q, want %q", got.Status, StatusStopped)
	}

	if _, err := GetServer(db, "missing"); err == nil {
		t.Error("expected an error for an unknown server id")
	}
}

func TestGetAllServers(t *testing.T) {
	db := newTestDB(t)
	newTestServer(t, db, "srv-1", 25565)
	newTestServer(t, db, "srv-2", 25566)

	servers, err := GetAllServers(db)
	if err != nil {
		t.Fatalf("GetAllServers: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("GetAllServers returned %d servers, want 2", len(servers))
	}
}

func TestPortExists(t *testing.T) {
	db := newTestDB(t)
	newTestServer(t, db, "srv-1", 25565)

	taken, err := PortExists(db, 25565)
	if err != nil {
		t.Fatalf("PortExists: %v", err)
	}
	if !taken {
		t.Error("PortExists(25565) = false, want true")
	}

	free, err := PortExists(db, 25599)
	if err != nil {
		t.Fatalf("PortExists: %v", err)
	}
	if free {
		t.Error("PortExists(25599) = true, want false")
	}
}

func TestCountRunningServers(t *testing.T) {
	db := newTestDB(t)
	newTestServer(t, db, "srv-1", 25565)
	newTestServer(t, db, "srv-2", 25566)

	count, err := CountRunningServers(db)
	if err != nil {
		t.Fatalf("CountRunningServers: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}

	if err := UpdateServerStatus(db, "srv-1", StatusRunning); err != nil {
		t.Fatalf("UpdateServerStatus: %v", err)
	}

	count, err = CountRunningServers(db)
	if err != nil {
		t.Fatalf("CountRunningServers: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestUpdateServerFields(t *testing.T) {
	db := newTestDB(t)
	newTestServer(t, db, "srv-1", 25565)

	started := time.Now().Truncate(time.Second)
	if err := UpdateServerLastStarted(db, "srv-1", started); err != nil {
		t.Fatalf("UpdateServerLastStarted: %v", err)
	}
	if err := UpdateServerVersion(db, "srv-1", "1.21.11", "paper"); err != nil {
		t.Fatalf("UpdateServerVersion: %v", err)
	}
	if err := UpdateServerLimits(db, "srv-1", 1.5, 4096); err != nil {
		t.Fatalf("UpdateServerLimits: %v", err)
	}
	if err := UpdateRestartSchedule(db, "srv-1", "0 4 * * *"); err != nil {
		t.Fatalf("UpdateRestartSchedule: %v", err)
	}
	if err := SetServerReady(db, "srv-1", true); err != nil {
		t.Fatalf("SetServerReady: %v", err)
	}

	got, err := GetServer(db, "srv-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "1.21.11" || got.Flavor != "paper" {
		t.Errorf("version/flavor = %q/%q, want 1.21.11/paper", got.Version, got.Flavor)
	}
	if got.CPULimit != 1.5 || got.MemoryLimitMB != 4096 {
		t.Errorf("limits = %v/%d, want 1.5/4096", got.CPULimit, got.MemoryLimitMB)
	}
	if got.RestartSchedule != "0 4 * * *" {
		t.Errorf("schedule = %q", got.RestartSchedule)
	}
	if !got.Ready {
		t.Error("Ready = false, want true")
	}
	if got.LastStartedAt == nil {
		t.Error("LastStartedAt was not recorded")
	}
}

func TestShareTokens(t *testing.T) {
	db := newTestDB(t)
	newTestServer(t, db, "srv-1", 25565)

	if err := SetShareToken(db, "srv-1", "token-abc"); err != nil {
		t.Fatalf("SetShareToken: %v", err)
	}

	got, err := GetServerByShareToken(db, "token-abc")
	if err != nil {
		t.Fatalf("GetServerByShareToken: %v", err)
	}
	if got.ID != "srv-1" {
		t.Errorf("resolved share token to %q, want srv-1", got.ID)
	}

	if _, err := GetServerByShareToken(db, "nope"); err == nil {
		t.Error("expected an error for an unknown share token")
	}
}

func TestDeleteServer(t *testing.T) {
	db := newTestDB(t)
	newTestServer(t, db, "srv-1", 25565)

	if err := DeleteServer(db, "srv-1"); err != nil {
		t.Fatalf("DeleteServer: %v", err)
	}
	if _, err := GetServer(db, "srv-1"); err == nil {
		t.Error("server still present after delete")
	}

	// The port must be free for reuse afterwards.
	taken, err := PortExists(db, 25565)
	if err != nil {
		t.Fatal(err)
	}
	if taken {
		t.Error("port still marked as taken after the server was deleted")
	}
}
