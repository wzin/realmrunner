package metrics

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"
)

type ServerPing struct {
	OnlinePlayers int      `json:"online_players"`
	MaxPlayers    int      `json:"max_players"`
	PlayerNames   []string `json:"player_names"`
	MOTD          string   `json:"motd"`
}

type pingResponse struct {
	Players struct {
		Max    int `json:"max"`
		Online int `json:"online"`
		Sample []struct {
			Name string `json:"name"`
			ID   string `json:"id"`
		} `json:"sample"`
	} `json:"players"`
	Description interface{} `json:"description"`
}

// QueryServerStatus sends a Minecraft Server List Ping to localhost:port
func QueryServerStatus(port int) (*ServerPing, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Send Handshake packet
	handshake := &bytes.Buffer{}
	writeVarInt(handshake, 0x00)       // Packet ID
	writeVarInt(handshake, -1)         // Protocol version (-1 for status)
	writeString(handshake, "127.0.0.1") // Server address
	binary.Write(handshake, binary.BigEndian, uint16(port)) // Port
	writeVarInt(handshake, 1)          // Next state: Status

	// Write handshake with length prefix
	writePacket(conn, handshake.Bytes())

	// Send Status Request packet
	statusReq := &bytes.Buffer{}
	writeVarInt(statusReq, 0x00) // Packet ID
	writePacket(conn, statusReq.Bytes())

	// Read Status Response
	packetData, err := readPacket(conn)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	reader := bytes.NewReader(packetData)

	// Read packet ID
	packetID, err := readVarInt(reader)
	if err != nil || packetID != 0x00 {
		return nil, fmt.Errorf("unexpected packet ID: %d", packetID)
	}

	// Read JSON string
	jsonStr, err := readString(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read JSON: %w", err)
	}

	// Parse response
	var resp pingResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	result := &ServerPing{
		OnlinePlayers: resp.Players.Online,
		MaxPlayers:    resp.Players.Max,
	}

	for _, p := range resp.Players.Sample {
		result.PlayerNames = append(result.PlayerNames, p.Name)
	}

	// Extract MOTD
	switch desc := resp.Description.(type) {
	case string:
		result.MOTD = desc
	case map[string]interface{}:
		if text, ok := desc["text"].(string); ok {
			result.MOTD = text
		}
	}

	return result, nil
}

func writeVarInt(w io.Writer, value int) {
	uval := uint32(value)
	for {
		b := byte(uval & 0x7F)
		uval >>= 7
		if uval != 0 {
			b |= 0x80
		}
		w.Write([]byte{b})
		if uval == 0 {
			break
		}
	}
}

func readVarInt(r io.Reader) (int, error) {
	var result int
	var shift uint
	buf := make([]byte, 1)
	for {
		if _, err := io.ReadFull(r, buf); err != nil {
			return 0, err
		}
		result |= int(buf[0]&0x7F) << shift
		if buf[0]&0x80 == 0 {
			break
		}
		shift += 7
		if shift >= 35 {
			return 0, fmt.Errorf("VarInt too big")
		}
	}
	return result, nil
}

func writeString(w io.Writer, s string) {
	writeVarInt(w, len(s))
	w.Write([]byte(s))
}

func readString(r io.Reader) (string, error) {
	length, err := readVarInt(r)
	if err != nil {
		return "", err
	}
	if length < 0 || length > 32767 {
		return "", fmt.Errorf("string too long: %d", length)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func writePacket(w io.Writer, data []byte) error {
	lenBuf := &bytes.Buffer{}
	writeVarInt(lenBuf, len(data))
	if _, err := w.Write(lenBuf.Bytes()); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

func readPacket(r io.Reader) ([]byte, error) {
	length, err := readVarInt(r)
	if err != nil {
		return nil, err
	}
	if length < 0 || length > 2097151 {
		return nil, fmt.Errorf("packet too large: %d", length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}
	return data, nil
}
