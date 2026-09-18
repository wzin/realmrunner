// Package rcon implements the Source RCON protocol used by Minecraft servers.
//
// RCON is a more reliable control channel than the process stdin pipe: it works
// for any running server, not only one this process started, it reports whether
// a command was actually delivered, and it returns the server's answer.
package rcon

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

// Packet types defined by the Source RCON protocol.
const (
	typeResponse = 0 // SERVERDATA_RESPONSE_VALUE
	typeCommand  = 2 // SERVERDATA_EXECCOMMAND
	typeAuth     = 3 // SERVERDATA_AUTH
	// The server answers an auth request with type 2, reusing the command id.
	typeAuthResponse = 2
)

const (
	// maxPacketSize guards against a malformed or hostile length prefix.
	maxPacketSize = 4096 + 16
	// trailerTimeout is how long to wait for additional packets once the first
	// response has arrived. Minecraft splits long output (such as a full player
	// list) across several packets with no end marker.
	trailerTimeout = 150 * time.Millisecond
)

// ErrAuthFailed is returned when the server rejects the RCON password.
var ErrAuthFailed = fmt.Errorf("rcon: authentication failed")

type Client struct {
	mu      sync.Mutex
	conn    net.Conn
	nextID  int32
	timeout time.Duration
}

// Dial connects to an RCON endpoint and authenticates.
func Dial(addr, password string, timeout time.Duration) (*Client, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("rcon: connect: %w", err)
	}

	c := &Client{conn: conn, nextID: 1, timeout: timeout}
	if err := c.authenticate(password); err != nil {
		conn.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) authenticate(password string) error {
	id := c.nextID
	c.nextID++

	if err := c.writePacket(id, typeAuth, password); err != nil {
		return err
	}

	// The server may send an empty SERVERDATA_RESPONSE_VALUE before the auth
	// response; skip it.
	for {
		c.conn.SetReadDeadline(time.Now().Add(c.timeout))
		respID, respType, _, err := c.readPacket()
		if err != nil {
			return fmt.Errorf("rcon: auth: %w", err)
		}
		if respType == typeResponse {
			continue
		}
		if respType != typeAuthResponse {
			return fmt.Errorf("rcon: unexpected auth response type %d", respType)
		}
		// An id of -1 means the password was rejected.
		if respID == -1 || respID != id {
			return ErrAuthFailed
		}
		return nil
	}
}

// Execute runs a console command and returns the server's response.
func (c *Client) Execute(command string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return "", fmt.Errorf("rcon: connection closed")
	}

	id := c.nextID
	c.nextID++

	if err := c.writePacket(id, typeCommand, command); err != nil {
		return "", err
	}

	var body strings.Builder
	c.conn.SetReadDeadline(time.Now().Add(c.timeout))
	respID, _, first, err := c.readPacket()
	if err != nil {
		return "", fmt.Errorf("rcon: read response: %w", err)
	}
	if respID != id {
		return "", fmt.Errorf("rcon: response id %d does not match request %d", respID, id)
	}
	body.WriteString(first)

	// Long output arrives as several packets; collect whatever follows quickly.
	for {
		c.conn.SetReadDeadline(time.Now().Add(trailerTimeout))
		_, _, more, err := c.readPacket()
		if err != nil {
			break
		}
		body.WriteString(more)
	}

	return body.String(), nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *Client) writePacket(id int32, packetType int32, body string) error {
	buf := &bytes.Buffer{}
	binary.Write(buf, binary.LittleEndian, id)
	binary.Write(buf, binary.LittleEndian, packetType)
	buf.WriteString(body)
	buf.WriteByte(0) // body terminator
	buf.WriteByte(0) // packet terminator

	out := &bytes.Buffer{}
	binary.Write(out, binary.LittleEndian, int32(buf.Len()))
	out.Write(buf.Bytes())

	c.conn.SetWriteDeadline(time.Now().Add(c.timeout))
	if _, err := c.conn.Write(out.Bytes()); err != nil {
		return fmt.Errorf("rcon: write: %w", err)
	}
	return nil
}

func (c *Client) readPacket() (id int32, packetType int32, body string, err error) {
	var length int32
	if err = binary.Read(c.conn, binary.LittleEndian, &length); err != nil {
		return 0, 0, "", err
	}
	if length < 10 || length > maxPacketSize {
		return 0, 0, "", fmt.Errorf("rcon: invalid packet length %d", length)
	}

	payload := make([]byte, length)
	if _, err = io.ReadFull(c.conn, payload); err != nil {
		return 0, 0, "", err
	}

	reader := bytes.NewReader(payload)
	binary.Read(reader, binary.LittleEndian, &id)
	binary.Read(reader, binary.LittleEndian, &packetType)

	// The remainder is the body plus two terminating null bytes.
	rest := payload[8:]
	rest = bytes.TrimRight(rest, "\x00")
	return id, packetType, string(rest), nil
}

// Execute opens a short-lived connection, runs one command and closes it. It
// suits callers that issue the occasional command and do not want to hold a
// connection open.
func Execute(addr, password, command string, timeout time.Duration) (string, error) {
	client, err := Dial(addr, password, timeout)
	if err != nil {
		return "", err
	}
	defer client.Close()

	return client.Execute(command)
}
