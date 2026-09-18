// Package mcproto implements the small part of the Minecraft protocol that
// RealmRunner needs: the handshake, the server list ping, and login
// disconnects. It is shared by the status poller and the sleep proxy.
package mcproto

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// Handshake states.
const (
	StateStatus = 1
	StateLogin  = 2
)

// WriteVarInt writes a Minecraft variable-length integer.
func WriteVarInt(w io.Writer, value int) error {
	uval := uint32(value)
	for {
		b := byte(uval & 0x7F)
		uval >>= 7
		if uval != 0 {
			b |= 0x80
		}
		if _, err := w.Write([]byte{b}); err != nil {
			return err
		}
		if uval == 0 {
			return nil
		}
	}
}

// ReadVarInt reads a Minecraft variable-length integer.
func ReadVarInt(r io.Reader) (int, error) {
	var result uint32
	var shift uint
	buf := make([]byte, 1)

	for {
		if _, err := io.ReadFull(r, buf); err != nil {
			return 0, err
		}
		result |= uint32(buf[0]&0x7F) << shift
		if buf[0]&0x80 == 0 {
			// VarInts are signed 32-bit values: clients send -1 as the
			// protocol version when they only want a status response.
			return int(int32(result)), nil
		}
		shift += 7
		if shift >= 35 {
			return 0, fmt.Errorf("mcproto: VarInt too big")
		}
	}
}

// WriteString writes a length-prefixed UTF-8 string.
func WriteString(w io.Writer, value string) error {
	if err := WriteVarInt(w, len(value)); err != nil {
		return err
	}
	_, err := io.WriteString(w, value)
	return err
}

// ReadString reads a length-prefixed UTF-8 string.
func ReadString(r io.Reader) (string, error) {
	length, err := ReadVarInt(r)
	if err != nil {
		return "", err
	}
	if length < 0 || length > maxStringLength {
		return "", fmt.Errorf("mcproto: string length %d out of range", length)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

const (
	maxStringLength = 32767
	// MaxPacketSize caps what a single packet may claim to be, so a hostile
	// client cannot make the proxy allocate arbitrary memory.
	MaxPacketSize = 2 * 1024 * 1024
)

// WritePacket frames a packet body with its length prefix.
func WritePacket(w io.Writer, body []byte) error {
	if err := WriteVarInt(w, len(body)); err != nil {
		return err
	}
	_, err := w.Write(body)
	return err
}

// ReadPacket reads one length-prefixed packet and returns its body.
func ReadPacket(r io.Reader) ([]byte, error) {
	length, err := ReadVarInt(r)
	if err != nil {
		return nil, err
	}
	if length < 0 || length > MaxPacketSize {
		return nil, fmt.Errorf("mcproto: packet length %d out of range", length)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}

// Handshake is the first packet a client sends.
type Handshake struct {
	ProtocolVersion int
	ServerAddress   string
	ServerPort      uint16
	NextState       int
}

// ReadHandshake parses a handshake packet body.
func ReadHandshake(body []byte) (*Handshake, error) {
	reader := bytes.NewReader(body)

	packetID, err := ReadVarInt(reader)
	if err != nil {
		return nil, err
	}
	if packetID != 0x00 {
		return nil, fmt.Errorf("mcproto: expected handshake packet, got id %d", packetID)
	}

	h := &Handshake{}
	if h.ProtocolVersion, err = ReadVarInt(reader); err != nil {
		return nil, err
	}
	if h.ServerAddress, err = ReadString(reader); err != nil {
		return nil, err
	}
	if err = binary.Read(reader, binary.BigEndian, &h.ServerPort); err != nil {
		return nil, err
	}
	if h.NextState, err = ReadVarInt(reader); err != nil {
		return nil, err
	}
	return h, nil
}

// EncodeHandshake builds a handshake packet body.
func EncodeHandshake(h *Handshake) []byte {
	buf := &bytes.Buffer{}
	WriteVarInt(buf, 0x00)
	WriteVarInt(buf, h.ProtocolVersion)
	WriteString(buf, h.ServerAddress)
	binary.Write(buf, binary.BigEndian, h.ServerPort)
	WriteVarInt(buf, h.NextState)
	return buf.Bytes()
}

// StatusResponse is the JSON document returned for a server list ping.
type StatusResponse struct {
	Version struct {
		Name     string `json:"name"`
		Protocol int    `json:"protocol"`
	} `json:"version"`
	Players struct {
		Max    int           `json:"max"`
		Online int           `json:"online"`
		Sample []interface{} `json:"sample"`
	} `json:"players"`
	Description struct {
		Text string `json:"text"`
	} `json:"description"`
}

// NewStatusResponse builds a status document that reports the given version
// name and MOTD. Echoing the client's own protocol number keeps the client from
// showing the server as incompatible while it is asleep.
func NewStatusResponse(versionName string, protocol, maxPlayers int, motd string) *StatusResponse {
	resp := &StatusResponse{}
	resp.Version.Name = versionName
	resp.Version.Protocol = protocol
	resp.Players.Max = maxPlayers
	resp.Players.Online = 0
	resp.Players.Sample = []interface{}{}
	resp.Description.Text = motd
	return resp
}

// WriteStatusPacket writes a status response packet (id 0x00) carrying json.
func WriteStatusPacket(w io.Writer, json string) error {
	body := &bytes.Buffer{}
	WriteVarInt(body, 0x00)
	WriteString(body, json)
	return WritePacket(w, body.Bytes())
}

// WritePongPacket echoes a ping payload back to the client (id 0x01).
func WritePongPacket(w io.Writer, payload []byte) error {
	body := &bytes.Buffer{}
	WriteVarInt(body, 0x01)
	body.Write(payload)
	return WritePacket(w, body.Bytes())
}

// WriteLoginDisconnect closes a login attempt with a message the player sees.
func WriteLoginDisconnect(w io.Writer, message string) error {
	body := &bytes.Buffer{}
	WriteVarInt(body, 0x00)
	// In the login state the reason is a JSON chat component.
	WriteString(body, fmt.Sprintf(`{"text":%s}`, quoteJSON(message)))
	return WritePacket(w, body.Bytes())
}

func quoteJSON(value string) string {
	out := &bytes.Buffer{}
	out.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"':
			out.WriteString(`\"`)
		case '\\':
			out.WriteString(`\\`)
		case '\n':
			out.WriteString(`\n`)
		default:
			out.WriteRune(r)
		}
	}
	out.WriteByte('"')
	return out.String()
}
