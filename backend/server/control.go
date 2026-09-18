package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/wzin/realmrunner/rcon"
)

const (
	// Derived ports keep allocation trivial while staying clear of the public
	// Minecraft range. Neither is published by the container.
	rconPortOffset     = 10000
	internalPortOffset = 20000

	rconTimeout = 5 * time.Second
)

// DerivedRCONPort is the loopback control port for a server's public port.
func DerivedRCONPort(port int) int { return port + rconPortOffset }

// DerivedInternalPort is where the Minecraft process listens when the proxy
// fronts the public port.
func DerivedInternalPort(port int) int { return port + internalPortOffset }

func generateRCONPassword() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate rcon password: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// ensureControlConfig gives a server RCON credentials if it has none (servers
// created before this feature existed) and writes the matching settings into
// server.properties. It returns the port the Minecraft process should bind.
func (m *Manager) ensureControlConfig(srv *Server) (int, error) {
	if srv.RCONPort == 0 || srv.RCONPassword == "" {
		password, err := generateRCONPassword()
		if err != nil {
			return 0, err
		}
		srv.RCONPort = DerivedRCONPort(srv.Port)
		srv.RCONPassword = password
		if err := SetRCONCredentials(m.db, srv.ID, srv.RCONPort, srv.RCONPassword); err != nil {
			return 0, err
		}
	}

	if srv.InternalPort == 0 {
		srv.InternalPort = DerivedInternalPort(srv.Port)
		if err := SetInternalPort(m.db, srv.ID, srv.InternalPort); err != nil {
			return 0, err
		}
	}

	// With auto-sleep the proxy owns the public port and the server listens on
	// the internal one; otherwise the server binds the public port directly.
	listenPort := srv.Port
	if srv.AutoSleep {
		listenPort = srv.InternalPort
	}

	propsPath := filepath.Join(m.getServerDir(srv.ID), "server.properties")
	updates := map[string]string{
		"server-port":           fmt.Sprintf("%d", listenPort),
		"enable-rcon":           "true",
		"rcon.port":             fmt.Sprintf("%d", srv.RCONPort),
		"rcon.password":         srv.RCONPassword,
		"broadcast-rcon-to-ops": "false",
		"enable-status":         "true",
	}
	if err := UpdateProperties(propsPath, updates); err != nil {
		return 0, fmt.Errorf("failed to write server.properties: %w", err)
	}

	return listenPort, nil
}

func rconAddr(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}

// RCONExecute runs a console command over RCON and returns the server's reply.
func (m *Manager) RCONExecute(id, command string) (string, error) {
	srv, err := GetServer(m.db, id)
	if err != nil {
		return "", err
	}
	if srv.Status != StatusRunning {
		return "", fmt.Errorf("server is not running")
	}
	if srv.RCONPort == 0 || srv.RCONPassword == "" {
		return "", fmt.Errorf("rcon is not configured for this server; restart it to enable")
	}

	return rcon.Execute(rconAddr(srv.RCONPort), srv.RCONPassword, command, rconTimeout)
}

// Players reports who is online, asking the server over RCON.
func (m *Manager) Players(id string) (*rcon.PlayerList, error) {
	response, err := m.RCONExecute(id, "list")
	if err != nil {
		return nil, err
	}
	return rcon.ParsePlayerList(response)
}

// PlayerCount is a convenience wrapper used by the idle watcher.
func (m *Manager) PlayerCount(id string) (int, error) {
	list, err := m.Players(id)
	if err != nil {
		return 0, err
	}
	return list.Online, nil
}

// KickPlayer removes a player from the server, with an optional reason.
func (m *Manager) KickPlayer(id, player, reason string) (string, error) {
	command := "kick " + player
	if reason = strings.TrimSpace(reason); reason != "" {
		command += " " + reason
	}
	return m.RCONExecute(id, command)
}

// BanPlayer bans a player, with an optional reason.
func (m *Manager) BanPlayer(id, player, reason string) (string, error) {
	command := "ban " + player
	if reason = strings.TrimSpace(reason); reason != "" {
		command += " " + reason
	}
	return m.RCONExecute(id, command)
}

// PardonPlayer lifts a ban.
func (m *Manager) PardonPlayer(id, player string) (string, error) {
	return m.RCONExecute(id, "pardon "+player)
}

// OpPlayer grants operator status.
func (m *Manager) OpPlayer(id, player string) (string, error) {
	return m.RCONExecute(id, "op "+player)
}

// DeopPlayer revokes operator status.
func (m *Manager) DeopPlayer(id, player string) (string, error) {
	return m.RCONExecute(id, "deop "+player)
}

// waitForRCON polls until the server accepts RCON connections, which is a
// better readiness signal than "the process started".
func (m *Manager) waitForRCON(srv *Server, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for time.Now().Before(deadline) {
		client, err := rcon.Dial(rconAddr(srv.RCONPort), srv.RCONPassword, 2*time.Second)
		if err == nil {
			client.Close()
			return nil
		}
		lastErr = err
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("server did not accept rcon connections within %s: %w", timeout, lastErr)
}
