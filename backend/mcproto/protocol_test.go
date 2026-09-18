package mcproto

import (
	"bytes"
	"strings"
	"testing"
)

func TestVarIntRoundTrip(t *testing.T) {
	values := []int{0, 1, 2, 127, 128, 255, 2097151, 2147483647, -1}

	for _, v := range values {
		buf := &bytes.Buffer{}
		if err := WriteVarInt(buf, v); err != nil {
			t.Fatalf("WriteVarInt(%d): %v", v, err)
		}
		got, err := ReadVarInt(buf)
		if err != nil {
			t.Fatalf("ReadVarInt(%d): %v", v, err)
		}
		if got != v {
			t.Errorf("round trip of %d produced %d", v, got)
		}
	}
}

func TestReadVarIntRejectsOverlongEncoding(t *testing.T) {
	// Six continuation bytes is more than a VarInt may use.
	data := bytes.NewReader([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})
	if _, err := ReadVarInt(data); err == nil {
		t.Error("expected an error for an overlong VarInt")
	}
}

func TestStringRoundTrip(t *testing.T) {
	for _, v := range []string{"", "localhost", "realmrunner.ziniewicz.eu", "ünïcödé ✅"} {
		buf := &bytes.Buffer{}
		if err := WriteString(buf, v); err != nil {
			t.Fatal(err)
		}
		got, err := ReadString(buf)
		if err != nil {
			t.Fatal(err)
		}
		if got != v {
			t.Errorf("round trip of %q produced %q", v, got)
		}
	}
}

func TestHandshakeRoundTrip(t *testing.T) {
	want := &Handshake{ProtocolVersion: 774, ServerAddress: "mc.example.com", ServerPort: 25565, NextState: StateLogin}

	got, err := ReadHandshake(EncodeHandshake(want))
	if err != nil {
		t.Fatalf("ReadHandshake: %v", err)
	}
	if *got != *want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestReadHandshakeRejectsOtherPackets(t *testing.T) {
	buf := &bytes.Buffer{}
	WriteVarInt(buf, 0x01) // not a handshake

	if _, err := ReadHandshake(buf.Bytes()); err == nil {
		t.Error("expected an error for a non-handshake packet")
	}
}

func TestPacketFraming(t *testing.T) {
	buf := &bytes.Buffer{}
	if err := WritePacket(buf, []byte{0x00, 0x01, 0x02}); err != nil {
		t.Fatal(err)
	}

	body, err := ReadPacket(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, []byte{0x00, 0x01, 0x02}) {
		t.Errorf("got %v", body)
	}
}

// A hostile length prefix must not make the proxy allocate unbounded memory.
func TestReadPacketRejectsHugeLength(t *testing.T) {
	buf := &bytes.Buffer{}
	WriteVarInt(buf, MaxPacketSize+1)

	if _, err := ReadPacket(buf); err == nil {
		t.Error("expected an error for an oversized packet")
	}
}

func TestWriteLoginDisconnectEscapesMessage(t *testing.T) {
	buf := &bytes.Buffer{}
	if err := WriteLoginDisconnect(buf, `still "starting"`+"\nreconnect"); err != nil {
		t.Fatal(err)
	}

	body, err := ReadPacket(buf)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(body)
	id, err := ReadVarInt(reader)
	if err != nil || id != 0x00 {
		t.Fatalf("packet id = %d, %v", id, err)
	}
	payload, err := ReadString(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, `\"starting\"`) || !strings.Contains(payload, `\n`) {
		t.Errorf("message was not escaped: %s", payload)
	}
}

func TestNewStatusResponseEchoesClientProtocol(t *testing.T) {
	resp := NewStatusResponse("26.3", 774, 20, "Sleeping")

	if resp.Version.Protocol != 774 {
		t.Errorf("protocol = %d, want the client's 774", resp.Version.Protocol)
	}
	if resp.Players.Online != 0 || resp.Players.Max != 20 {
		t.Errorf("players = %+v", resp.Players)
	}
	if resp.Description.Text != "Sleeping" {
		t.Errorf("description = %q", resp.Description.Text)
	}
	if resp.Players.Sample == nil {
		t.Error("sample must be an empty list, not null")
	}
}
