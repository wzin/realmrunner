package rcon

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeServer is a minimal Source RCON server used to exercise the client.
type fakeServer struct {
	listener net.Listener
	password string
	// responses maps a command to the chunks it is answered with, letting a
	// test reproduce Minecraft's split responses.
	responses map[string][]string
}

func newFakeServer(t *testing.T, password string, responses map[string][]string) *fakeServer {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	s := &fakeServer{listener: ln, password: password, responses: responses}
	go s.serve()
	t.Cleanup(func() { ln.Close() })
	return s
}

func (s *fakeServer) addr() string { return s.listener.Addr().String() }

func (s *fakeServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *fakeServer) handle(conn net.Conn) {
	defer conn.Close()

	authenticated := false
	for {
		id, packetType, body, err := readTestPacket(conn)
		if err != nil {
			return
		}

		switch packetType {
		case typeAuth:
			if body != s.password {
				// A rejected password is signalled with id -1.
				writeTestPacket(conn, -1, typeAuthResponse, "")
				return
			}
			authenticated = true
			writeTestPacket(conn, id, typeAuthResponse, "")
		case typeCommand:
			if !authenticated {
				writeTestPacket(conn, id, typeResponse, "unauthenticated")
				continue
			}
			chunks, ok := s.responses[body]
			if !ok {
				chunks = []string{"Unknown command"}
			}
			for _, chunk := range chunks {
				writeTestPacket(conn, id, typeResponse, chunk)
			}
		}
	}
}

func writeTestPacket(w io.Writer, id, packetType int32, body string) {
	buf := &bytes.Buffer{}
	binary.Write(buf, binary.LittleEndian, id)
	binary.Write(buf, binary.LittleEndian, packetType)
	buf.WriteString(body)
	buf.WriteByte(0)
	buf.WriteByte(0)

	binary.Write(w, binary.LittleEndian, int32(buf.Len()))
	w.Write(buf.Bytes())
}

func readTestPacket(r io.Reader) (id, packetType int32, body string, err error) {
	var length int32
	if err = binary.Read(r, binary.LittleEndian, &length); err != nil {
		return
	}
	payload := make([]byte, length)
	if _, err = io.ReadFull(r, payload); err != nil {
		return
	}
	reader := bytes.NewReader(payload)
	binary.Read(reader, binary.LittleEndian, &id)
	binary.Read(reader, binary.LittleEndian, &packetType)
	return id, packetType, string(bytes.TrimRight(payload[8:], "\x00")), nil
}

func TestDialAndExecute(t *testing.T) {
	srv := newFakeServer(t, "secret", map[string][]string{
		"list": {"There are 2 of a max of 20 players online: Alice, Bob"},
	})

	client, err := Dial(srv.addr(), "secret", time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	got, err := client.Execute("list")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(got, "Alice, Bob") {
		t.Errorf("Execute returned %q", got)
	}
}

// Several commands in a row must keep working: the trailer read deadline must
// not leak into the next command.
func TestExecuteRepeated(t *testing.T) {
	srv := newFakeServer(t, "secret", map[string][]string{
		"list": {"There are 0 of a max of 20 players online:"},
		"seed": {"Seed: [123456]"},
	})

	client, err := Dial(srv.addr(), "secret", time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	for i := 0; i < 3; i++ {
		if _, err := client.Execute("list"); err != nil {
			t.Fatalf("Execute #%d: %v", i, err)
		}
	}
	got, err := client.Execute("seed")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(got, "123456") {
		t.Errorf("Execute returned %q", got)
	}
}

// Minecraft splits long output across packets; the client must join them.
func TestExecuteMultiPacketResponse(t *testing.T) {
	srv := newFakeServer(t, "secret", map[string][]string{
		"list": {"There are 3 of a max of 20 players online: ", "Alice, Bob, ", "Carol"},
	})

	client, err := Dial(srv.addr(), "secret", time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	got, err := client.Execute("list")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.HasSuffix(got, "Alice, Bob, Carol") {
		t.Errorf("Execute returned %q, want the joined response", got)
	}
}

func TestDialWrongPassword(t *testing.T) {
	srv := newFakeServer(t, "secret", nil)

	if _, err := Dial(srv.addr(), "wrong", time.Second); err != ErrAuthFailed {
		t.Errorf("Dial with a wrong password returned %v, want ErrAuthFailed", err)
	}
}

func TestDialUnreachable(t *testing.T) {
	// Port 1 on loopback refuses connections.
	if _, err := Dial("127.0.0.1:1", "secret", 500*time.Millisecond); err == nil {
		t.Error("expected an error when the server is not listening")
	}
}

func TestExecuteAfterClose(t *testing.T) {
	srv := newFakeServer(t, "secret", nil)

	client, err := Dial(srv.addr(), "secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Errorf("second Close returned %v, want nil", err)
	}
	if _, err := client.Execute("list"); err == nil {
		t.Error("Execute on a closed client should fail")
	}
}

func TestExecuteHelper(t *testing.T) {
	srv := newFakeServer(t, "secret", map[string][]string{
		"save-all": {"Saved the game"},
	})

	got, err := Execute(srv.addr(), "secret", "save-all", time.Second)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(got, "Saved the game") {
		t.Errorf("Execute returned %q", got)
	}
}
