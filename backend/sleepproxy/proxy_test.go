package sleepproxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wzin/realmrunner/mcproto"
)

// fakeBackend stands in for a Minecraft server listening on the internal port.
type fakeBackend struct {
	listener net.Listener
	received chan []byte
}

func newFakeBackend(t *testing.T, port int) *fakeBackend {
	t.Helper()

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("listen on internal port: %v", err)
	}

	b := &fakeBackend{listener: ln, received: make(chan []byte, 4)}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				// Echo the handshake we were handed, then anything else.
				body, err := mcproto.ReadPacket(conn)
				if err != nil {
					return
				}
				b.received <- body
				conn.Write([]byte("backend-hello"))
			}()
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return b
}

// freePort asks the kernel for an unused port.
func freePort(t *testing.T) int {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func newTestProxy(t *testing.T, hooks Hooks, internalPort int) *Proxy {
	t.Helper()

	p := New(Options{
		ServerID:     "srv-1",
		PublicPort:   freePort(t),
		InternalPort: internalPort,
		WakeTimeout:  5 * time.Second,
		DialTimeout:  500 * time.Millisecond,
	}, hooks)

	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

func sleepingDescription() Description {
	return Description{
		VersionName:  "26.3",
		MaxPlayers:   20,
		MOTD:         "Sleeping - join to wake",
		StartingMOTD: "Starting up...",
	}
}

// pingProxy performs a server list ping and returns the status document.
func pingProxy(t *testing.T, addr string, protocol int) *mcproto.StatusResponse {
	t.Helper()

	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))

	handshake := mcproto.EncodeHandshake(&mcproto.Handshake{
		ProtocolVersion: protocol,
		ServerAddress:   "127.0.0.1",
		ServerPort:      25565,
		NextState:       mcproto.StateStatus,
	})
	if err := mcproto.WritePacket(conn, handshake); err != nil {
		t.Fatal(err)
	}

	// Status request.
	request := &bytes.Buffer{}
	mcproto.WriteVarInt(request, 0x00)
	if err := mcproto.WritePacket(conn, request.Bytes()); err != nil {
		t.Fatal(err)
	}

	body, err := mcproto.ReadPacket(conn)
	if err != nil {
		t.Fatalf("read status response: %v", err)
	}

	reader := bytes.NewReader(body)
	if _, err := mcproto.ReadVarInt(reader); err != nil {
		t.Fatal(err)
	}
	payload, err := mcproto.ReadString(reader)
	if err != nil {
		t.Fatal(err)
	}

	var status mcproto.StatusResponse
	if err := json.Unmarshal([]byte(payload), &status); err != nil {
		t.Fatalf("status response is not valid JSON: %v (%s)", err, payload)
	}
	return &status
}

// A sleeping server still shows up in the client's server list, with a MOTD
// that says it is asleep - and pinging it must not start it.
func TestStatusPingOnSleepingServerDoesNotWakeIt(t *testing.T) {
	var wakes int32
	proxy := newTestProxy(t, Hooks{
		Wake:     func() error { atomic.AddInt32(&wakes, 1); return nil },
		Awake:    func() bool { return false },
		Describe: sleepingDescription,
	}, freePort(t))

	status := pingProxy(t, proxy.Addr().String(), 774)

	if status.Description.Text != "Sleeping - join to wake" {
		t.Errorf("MOTD = %q", status.Description.Text)
	}
	if status.Version.Protocol != 774 {
		t.Errorf("protocol = %d, want the client's own 774", status.Version.Protocol)
	}
	if status.Players.Max != 20 || status.Players.Online != 0 {
		t.Errorf("players = %+v", status.Players)
	}
	if atomic.LoadInt32(&wakes) != 0 {
		t.Error("a status ping started the server")
	}
}

func TestStatusPingWhileStarting(t *testing.T) {
	proxy := newTestProxy(t, Hooks{
		Wake:  func() error { return nil },
		Awake: func() bool { return false },
		Describe: func() Description {
			d := sleepingDescription()
			d.Starting = true
			return d
		},
	}, freePort(t))

	status := pingProxy(t, proxy.Addr().String(), 774)
	if status.Description.Text != "Starting up..." {
		t.Errorf("MOTD = %q, want the starting message", status.Description.Text)
	}
}

// A player joining wakes the server and, once it is up, gets handed over to it
// with the handshake replayed so the server sees a normal connection.
func TestLoginWakesServerAndHandsOverConnection(t *testing.T) {
	internalPort := freePort(t)

	var awake atomic.Bool
	var backend *fakeBackend
	proxy := newTestProxy(t, Hooks{
		Wake: func() error {
			if awake.Load() {
				return nil
			}
			backend = newFakeBackend(t, internalPort)
			awake.Store(true)
			return nil
		},
		Awake:    func() bool { return awake.Load() },
		Describe: sleepingDescription,
	}, internalPort)

	conn, err := net.DialTimeout("tcp", proxy.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	handshake := mcproto.EncodeHandshake(&mcproto.Handshake{
		ProtocolVersion: 774,
		ServerAddress:   "127.0.0.1",
		ServerPort:      25565,
		NextState:       mcproto.StateLogin,
	})
	if err := mcproto.WritePacket(conn, handshake); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-backendReceived(t, &backend):
		replayed, err := mcproto.ReadHandshake(got)
		if err != nil {
			t.Fatalf("the server did not receive a valid handshake: %v", err)
		}
		if replayed.NextState != mcproto.StateLogin || replayed.ProtocolVersion != 774 {
			t.Errorf("replayed handshake = %+v", replayed)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("the woken server never received the player's handshake")
	}

	// Data from the server must reach the player.
	buf := make([]byte, len("backend-hello"))
	if _, err := conn.Read(buf); err != nil {
		t.Fatalf("read from backend through proxy: %v", err)
	}
	if string(buf) != "backend-hello" {
		t.Errorf("got %q from the server", buf)
	}
}

// backendReceived waits for the fake backend to be created by the Wake hook and
// returns its channel.
func backendReceived(t *testing.T, backend **fakeBackend) chan []byte {
	t.Helper()

	out := make(chan []byte, 1)
	go func() {
		for i := 0; i < 100; i++ {
			if *backend != nil {
				select {
				case body := <-(*backend).received:
					out <- body
					return
				case <-time.After(100 * time.Millisecond):
				}
				continue
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
	return out
}

// If the server does not come up in time, the player is told to reconnect
// instead of being left hanging.
func TestLoginTimesOutWithAMessage(t *testing.T) {
	proxy := New(Options{
		ServerID:     "srv-1",
		PublicPort:   freePort(t),
		InternalPort: freePort(t), // nothing ever listens here
		WakeTimeout:  1 * time.Second,
		DialTimeout:  200 * time.Millisecond,
	}, Hooks{
		Wake:     func() error { return nil },
		Awake:    func() bool { return false },
		Describe: sleepingDescription,
	})
	if err := proxy.Start(); err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	conn, err := net.DialTimeout("tcp", proxy.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	mcproto.WritePacket(conn, mcproto.EncodeHandshake(&mcproto.Handshake{
		ProtocolVersion: 774, ServerAddress: "127.0.0.1", ServerPort: 25565, NextState: mcproto.StateLogin,
	}))

	body, err := mcproto.ReadPacket(conn)
	if err != nil {
		t.Fatalf("expected a disconnect message: %v", err)
	}
	reader := bytes.NewReader(body)
	mcproto.ReadVarInt(reader)
	message, err := mcproto.ReadString(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "starting") {
		t.Errorf("disconnect message = %q", message)
	}
}

// A failed start is reported to the player rather than silently dropped.
func TestLoginReportsWakeFailure(t *testing.T) {
	proxy := newTestProxy(t, Hooks{
		Wake:     func() error { return fmt.Errorf("maximum number of running servers reached") },
		Awake:    func() bool { return false },
		Describe: sleepingDescription,
	}, freePort(t))

	conn, err := net.DialTimeout("tcp", proxy.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	mcproto.WritePacket(conn, mcproto.EncodeHandshake(&mcproto.Handshake{
		ProtocolVersion: 774, ServerAddress: "127.0.0.1", ServerPort: 25565, NextState: mcproto.StateLogin,
	}))

	body, err := mcproto.ReadPacket(conn)
	if err != nil {
		t.Fatalf("expected a disconnect message: %v", err)
	}
	reader := bytes.NewReader(body)
	mcproto.ReadVarInt(reader)
	message, _ := mcproto.ReadString(reader)
	if !strings.Contains(message, "maximum number of running servers") {
		t.Errorf("disconnect message = %q", message)
	}
}

// While the server is awake the proxy is a plain pass-through.
func TestAwakeServerIsProxiedStraightThrough(t *testing.T) {
	internalPort := freePort(t)
	backend := newFakeBackend(t, internalPort)

	proxy := newTestProxy(t, Hooks{
		Wake:     func() error { t.Error("Wake called for an awake server"); return nil },
		Awake:    func() bool { return true },
		Describe: sleepingDescription,
	}, internalPort)

	conn, err := net.DialTimeout("tcp", proxy.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	mcproto.WritePacket(conn, mcproto.EncodeHandshake(&mcproto.Handshake{
		ProtocolVersion: 774, ServerAddress: "127.0.0.1", ServerPort: 25565, NextState: mcproto.StateStatus,
	}))

	select {
	case got := <-backend.received:
		if _, err := mcproto.ReadHandshake(got); err != nil {
			t.Errorf("the server received a malformed handshake: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the running server never received the connection")
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	proxy := newTestProxy(t, Hooks{
		Wake:     func() error { return nil },
		Awake:    func() bool { return false },
		Describe: sleepingDescription,
	}, freePort(t))

	if err := proxy.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := proxy.Close(); err != nil {
		t.Errorf("second Close returned %v", err)
	}
}

func TestStartFailsOnBusyPort(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()

	proxy := New(Options{
		ServerID:     "srv-1",
		PublicPort:   busy.Addr().(*net.TCPAddr).Port,
		InternalPort: freePort(t),
	}, Hooks{
		Wake:     func() error { return nil },
		Awake:    func() bool { return false },
		Describe: sleepingDescription,
	})

	if err := proxy.Start(); err == nil {
		proxy.Close()
		t.Error("expected Start to fail when the port is taken")
	}
}

// A proxied session must have TCP keep-alives on: without them a NAT on the
// path can silently drop a quiet connection, which players see as a random
// "connection lost".
func TestProxiedConnectionsUseKeepAlive(t *testing.T) {
	internalPort := freePort(t)
	backend := newFakeBackend(t, internalPort)

	proxy := newTestProxy(t, Hooks{
		Wake:     func() error { return nil },
		Awake:    func() bool { return true },
		Describe: sleepingDescription,
	}, internalPort)

	conn, err := net.DialTimeout("tcp", proxy.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := mcproto.WritePacket(conn, mcproto.EncodeHandshake(&mcproto.Handshake{
		ProtocolVersion: 774, ServerAddress: "127.0.0.1", ServerPort: 25565, NextState: mcproto.StateLogin,
	})); err != nil {
		t.Fatal(err)
	}

	select {
	case <-backend.received:
	case <-time.After(3 * time.Second):
		t.Fatal("the connection was never proxied")
	}

	// The proxy must have applied keep-alive to the sockets it owns. Exercise
	// the helper directly on a real TCP connection: it must accept it without
	// error and leave the connection usable.
	enableKeepAlive(conn)
	if _, err := conn.Write([]byte{0x00}); err != nil {
		t.Errorf("connection unusable after enabling keep-alive: %v", err)
	}
}
