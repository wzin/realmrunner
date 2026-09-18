// Package sleepproxy holds a sleeping server's port open, answers the server
// list ping while it is down, and starts it when a player tries to join.
//
// While auto-sleep is enabled for a server, the Minecraft process listens on an
// internal port and this proxy owns the public one, so players always connect
// to the same address whether the server is awake or not.
package sleepproxy

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/wzin/realmrunner/mcproto"
)

// Hooks is what the proxy needs from the rest of the application.
type Hooks struct {
	// Wake starts the server. It may be called more than once; it must be safe
	// to call while the server is already starting.
	Wake func() error
	// Awake reports whether the Minecraft server is currently running.
	Awake func() bool
	// Describe supplies what to show in the client's server list.
	Describe func() Description
}

// Description is what a client sees while the server sleeps.
type Description struct {
	VersionName string
	MaxPlayers  int
	MOTD        string
	// StartingMOTD is shown once the server is on its way up.
	StartingMOTD string
	Starting     bool
}

// Options configure a proxy instance.
type Options struct {
	ServerID     string
	PublicPort   int
	InternalPort int
	// WakeTimeout is how long a joining player's connection is held while the
	// server starts before they are asked to reconnect.
	WakeTimeout time.Duration
	// DialTimeout bounds each attempt to reach the started server.
	DialTimeout time.Duration
}

type Proxy struct {
	opts     Options
	hooks    Hooks
	listener net.Listener

	mu     sync.Mutex
	closed bool
	conns  sync.WaitGroup
}

const (
	defaultWakeTimeout = 90 * time.Second
	defaultDialTimeout = 2 * time.Second
	// handshakeTimeout bounds how long a client may take to send its handshake.
	handshakeTimeout = 10 * time.Second
	// keepAlivePeriod is short enough to hold a NAT mapping open on a home
	// router, which is where long-lived game connections usually get dropped.
	keepAlivePeriod = 30 * time.Second
)

// New creates a proxy. Call Start to begin listening.
func New(opts Options, hooks Hooks) *Proxy {
	if opts.WakeTimeout <= 0 {
		opts.WakeTimeout = defaultWakeTimeout
	}
	if opts.DialTimeout <= 0 {
		opts.DialTimeout = defaultDialTimeout
	}
	return &Proxy{opts: opts, hooks: hooks}
}

// Start binds the public port and serves connections in the background.
func (p *Proxy) Start() error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", p.opts.PublicPort))
	if err != nil {
		return fmt.Errorf("sleepproxy: listen on %d: %w", p.opts.PublicPort, err)
	}

	p.mu.Lock()
	p.listener = listener
	p.mu.Unlock()

	go p.acceptLoop(listener)
	log.Printf("Sleep proxy listening on port %d for server %s", p.opts.PublicPort, p.opts.ServerID)
	return nil
}

// Addr is the address the proxy listens on, useful in tests.
func (p *Proxy) Addr() net.Addr {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.listener == nil {
		return nil
	}
	return p.listener.Addr()
}

// Close stops listening and waits for in-flight connections to finish.
func (p *Proxy) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	listener := p.listener
	p.mu.Unlock()

	var err error
	if listener != nil {
		err = listener.Close()
	}
	p.conns.Wait()
	return err
}

func (p *Proxy) isClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

func (p *Proxy) acceptLoop(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			if p.isClosed() {
				return
			}
			log.Printf("Sleep proxy accept error on port %d: %v", p.opts.PublicPort, err)
			return
		}

		p.conns.Add(1)
		go func() {
			defer p.conns.Done()
			defer conn.Close()
			if err := p.handle(conn); err != nil && !isDisconnect(err) {
				log.Printf("Sleep proxy connection error (server %s): %v", p.opts.ServerID, err)
			}
		}()
	}
}

func (p *Proxy) handle(conn net.Conn) error {
	conn.SetReadDeadline(time.Now().Add(handshakeTimeout))

	body, err := mcproto.ReadPacket(conn)
	if err != nil {
		return fmt.Errorf("read handshake: %w", err)
	}
	handshake, err := mcproto.ReadHandshake(body)
	if err != nil {
		return err
	}
	conn.SetReadDeadline(time.Time{})

	// The server may be up already: just pass everything through.
	if p.hooks.Awake() {
		return p.pipeToServer(conn, body)
	}

	switch handshake.NextState {
	case mcproto.StateStatus:
		return p.serveStatus(conn, handshake)
	case mcproto.StateLogin:
		return p.serveWake(conn, body)
	default:
		return nil
	}
}

// serveStatus answers the server list ping for a sleeping server.
func (p *Proxy) serveStatus(conn net.Conn, handshake *mcproto.Handshake) error {
	desc := p.hooks.Describe()

	motd := desc.MOTD
	if desc.Starting {
		motd = desc.StartingMOTD
	}

	status := mcproto.NewStatusResponse(desc.VersionName, handshake.ProtocolVersion, desc.MaxPlayers, motd)
	encoded, err := json.Marshal(status)
	if err != nil {
		return err
	}

	for {
		conn.SetReadDeadline(time.Now().Add(handshakeTimeout))
		body, err := mcproto.ReadPacket(conn)
		if err != nil {
			return nil // the client is done
		}
		if len(body) == 0 {
			continue
		}

		switch body[0] {
		case 0x00: // status request
			if err := mcproto.WriteStatusPacket(conn, string(encoded)); err != nil {
				return err
			}
		case 0x01: // ping, echoed back verbatim
			if err := mcproto.WritePongPacket(conn, body[1:]); err != nil {
				return err
			}
			return nil
		default:
			return nil
		}
	}
}

// serveWake starts the server and holds the player's connection until it is
// ready, then hands the connection over.
func (p *Proxy) serveWake(conn net.Conn, handshakeBody []byte) error {
	log.Printf("Player connection woke server %s", p.opts.ServerID)

	if err := p.hooks.Wake(); err != nil {
		log.Printf("Waking server %s failed: %v", p.opts.ServerID, err)
		return mcproto.WriteLoginDisconnect(conn, "This realm could not be started:\n"+err.Error())
	}

	deadline := time.Now().Add(p.opts.WakeTimeout)
	for time.Now().Before(deadline) {
		if p.isClosed() {
			return nil
		}
		if backend, err := p.dialServer(); err == nil {
			return p.pipe(conn, backend, handshakeBody)
		}
		time.Sleep(time.Second)
	}

	return mcproto.WriteLoginDisconnect(conn,
		"This realm is still starting up.\nPlease reconnect in a moment.")
}

// pipeToServer forwards a connection to an already running server.
func (p *Proxy) pipeToServer(conn net.Conn, handshakeBody []byte) error {
	backend, err := p.dialServer()
	if err != nil {
		return mcproto.WriteLoginDisconnect(conn, "This realm is not reachable right now.")
	}
	return p.pipe(conn, backend, handshakeBody)
}

func (p *Proxy) dialServer() (net.Conn, error) {
	return net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", p.opts.InternalPort), p.opts.DialTimeout)
}

// pipe replays the handshake the proxy already consumed and then copies bytes
// in both directions until either side closes.
func (p *Proxy) pipe(client, backend net.Conn, handshakeBody []byte) error {
	defer backend.Close()

	// A proxied session is a long-lived connection that can sit quiet between
	// packets. Keep-alives stop a NAT or firewall on the path from silently
	// dropping it, and let this side notice a dead peer instead of holding the
	// player's slot open.
	enableKeepAlive(client)
	enableKeepAlive(backend)

	if err := mcproto.WritePacket(backend, handshakeBody); err != nil {
		return fmt.Errorf("replay handshake: %w", err)
	}

	done := make(chan struct{}, 2)
	go func() {
		io.Copy(backend, client)
		backend.SetReadDeadline(time.Now())
		done <- struct{}{}
	}()
	go func() {
		io.Copy(client, backend)
		client.SetReadDeadline(time.Now())
		done <- struct{}{}
	}()

	<-done
	return nil
}

// enableKeepAlive turns on TCP keep-alives for a proxied connection.
func enableKeepAlive(conn net.Conn) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return
	}
	tcpConn.SetKeepAlive(true)
	tcpConn.SetKeepAlivePeriod(keepAlivePeriod)
	// Minecraft is latency-sensitive and its packets are small; batching them
	// adds delay for no gain.
	tcpConn.SetNoDelay(true)
}

func isDisconnect(err error) bool {
	if err == nil {
		return true
	}
	if err == io.EOF {
		return true
	}
	var netErr net.Error
	if ok := asNetError(err, &netErr); ok && netErr.Timeout() {
		return true
	}
	return false
}

func asNetError(err error, target *net.Error) bool {
	if e, ok := err.(net.Error); ok {
		*target = e
		return true
	}
	return false
}
