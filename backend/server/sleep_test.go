package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/wzin/realmrunner/config"
	"github.com/wzin/realmrunner/mcproto"
	"github.com/wzin/realmrunner/minecraft"
)

func decodeStatus(body []byte) (*mcproto.StatusResponse, error) {
	reader := bytes.NewReader(body)
	if _, err := mcproto.ReadVarInt(reader); err != nil {
		return nil, err
	}
	payload, err := mcproto.ReadString(reader)
	if err != nil {
		return nil, err
	}
	var status mcproto.StatusResponse
	if err := json.Unmarshal([]byte(payload), &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// enabling auto-sleep must hold the public port and report the server as
// sleeping, so players see the realm and can wake it.
func TestSetAutoSleepHoldsPortAndReportsSleeping(t *testing.T) {
	m := newWideManager(t)

	port := freeTestPort(t)
	srv, err := m.CreateServer("sleepy", "26.3", "vanilla", port)
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}

	if err := m.SetAutoSleep(srv.ID, true, 5); err != nil {
		t.Fatalf("SetAutoSleep: %v", err)
	}
	t.Cleanup(m.Shutdown)

	got, err := m.GetServer(srv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusSleeping {
		t.Errorf("status = %q, want %q", got.Status, StatusSleeping)
	}
	if !got.AutoSleep || got.IdleTimeoutMin != 5 {
		t.Errorf("auto-sleep settings not stored: %+v", got)
	}

	// The Minecraft process must be moved off the public port.
	props, err := ReadProperties(m.GetServerDir(srv.ID) + "/server.properties")
	if err != nil {
		t.Fatal(err)
	}
	if props["server-port"] != fmt.Sprintf("%d", got.InternalPort) {
		t.Errorf("server-port = %q, want the internal port %d", props["server-port"], got.InternalPort)
	}

	// A client must be able to ping the sleeping realm on the public port.
	status := pingSleepingServer(t, port)
	if status.Version.Name != "26.3" {
		t.Errorf("ping reported version %q", status.Version.Name)
	}
	if status.Players.Online != 0 {
		t.Errorf("ping reported %d players online", status.Players.Online)
	}

	// Disabling it gives the port back and returns the server to stopped.
	if err := m.SetAutoSleep(srv.ID, false, 5); err != nil {
		t.Fatalf("SetAutoSleep(false): %v", err)
	}
	got, _ = m.GetServer(srv.ID)
	if got.Status != StatusStopped {
		t.Errorf("status = %q, want %q", got.Status, StatusStopped)
	}
	props, _ = ReadProperties(m.GetServerDir(srv.ID) + "/server.properties")
	if props["server-port"] != fmt.Sprintf("%d", port) {
		t.Errorf("server-port = %q, want the public port %d back", props["server-port"], port)
	}
}

// Auto-sleep cannot be toggled underneath a running server, because the
// Minecraft process would have to move between ports.
func TestSetAutoSleepRefusedWhileRunning(t *testing.T) {
	m := newWideManager(t)

	srv, err := m.CreateServer("busy", "26.3", "vanilla", freeTestPort(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := UpdateServerStatus(m.GetDB(), srv.ID, StatusRunning); err != nil {
		t.Fatal(err)
	}

	if err := m.SetAutoSleep(srv.ID, true, 5); err == nil {
		t.Error("expected auto-sleep to be refused for a running server")
	}
}

func TestSleepRequiresAutoSleepEnabled(t *testing.T) {
	m := newWideManager(t)

	srv, err := m.CreateServer("plain", "26.3", "vanilla", freeTestPort(t))
	if err != nil {
		t.Fatal(err)
	}

	if err := m.Sleep(srv.ID); err == nil {
		t.Error("expected Sleep to be refused without auto-sleep")
	}
}

// Servers configured to sleep get their port held again when RealmRunner
// restarts, and a stopped one is shown as sleeping.
func TestStartSleepProxiesOnBoot(t *testing.T) {
	m := newWideManager(t)

	port := freeTestPort(t)
	srv, err := m.CreateServer("sleepy", "26.3", "vanilla", port)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetAutoSleep(m.GetDB(), srv.ID, true, 5); err != nil {
		t.Fatal(err)
	}

	m.StartSleepProxies()
	t.Cleanup(m.Shutdown)

	got, _ := m.GetServer(srv.ID)
	if got.Status != StatusSleeping {
		t.Errorf("status after boot = %q, want %q", got.Status, StatusSleeping)
	}

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if err != nil {
		t.Fatalf("the port was not held after boot: %v", err)
	}
	conn.Close()
}

// newWideManager is a manager whose port range covers the ephemeral ports the
// tests bind, so a real listener can be created.
func newWideManager(t *testing.T) *Manager {
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
		PortRange:  config.PortRange{Min: 1024, Max: 65535},
	}
	return NewManager(db, cfg, nil, minecraft.NewRegistry(), nil)
}

func freeTestPort(t *testing.T) int {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func pingSleepingServer(t *testing.T, port int) *mcproto.StatusResponse {
	t.Helper()

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("dial sleeping server: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))

	handshake := mcproto.EncodeHandshake(&mcproto.Handshake{
		ProtocolVersion: 774, ServerAddress: "127.0.0.1", ServerPort: uint16(port), NextState: mcproto.StateStatus,
	})
	if err := mcproto.WritePacket(conn, handshake); err != nil {
		t.Fatal(err)
	}
	request := []byte{0x00}
	if err := mcproto.WritePacket(conn, request); err != nil {
		t.Fatal(err)
	}

	body, err := mcproto.ReadPacket(conn)
	if err != nil {
		t.Fatalf("read status: %v", err)
	}

	status, err := decodeStatus(body)
	if err != nil {
		t.Fatal(err)
	}
	return status
}
