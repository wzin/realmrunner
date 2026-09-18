package server

import (
	"fmt"
	"log"
	"time"

	"github.com/wzin/realmrunner/sleepproxy"
)

const (
	// idleCheckInterval is how often a sleeping-capable server is asked how
	// many players are online.
	idleCheckInterval = 60 * time.Second
	// defaultIdleTimeoutMin applies when a server has no timeout configured.
	defaultIdleTimeoutMin = 15
)

// sleepState tracks a server between idle checks.
type sleepState struct {
	emptySince time.Time
}

// StartSleepProxies brings up a proxy for every server with auto-sleep enabled
// and starts the idle watcher. It is called once at boot.
func (m *Manager) StartSleepProxies() {
	servers, err := GetAllServers(m.db)
	if err != nil {
		log.Printf("Auto-sleep: cannot list servers: %v", err)
		return
	}

	for _, srv := range servers {
		if !srv.AutoSleep {
			continue
		}
		if err := m.ensureSleepProxy(srv); err != nil {
			log.Printf("Auto-sleep: %v", err)
			continue
		}
		// A stopped server with auto-sleep on is asleep, not simply stopped:
		// its port is held and a player can wake it.
		if srv.Status == StatusStopped {
			UpdateServerStatus(m.db, srv.ID, StatusSleeping)
		}
	}

	go m.idleWatcher()
}

// ensureSleepProxy starts the proxy for a server if it is not already running.
func (m *Manager) ensureSleepProxy(srv *Server) error {
	m.mu.Lock()
	if m.proxies == nil {
		m.proxies = make(map[string]*sleepproxy.Proxy)
	}
	if _, exists := m.proxies[srv.ID]; exists {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	if srv.InternalPort == 0 {
		srv.InternalPort = DerivedInternalPort(srv.Port)
		if err := SetInternalPort(m.db, srv.ID, srv.InternalPort); err != nil {
			return err
		}
	}

	id := srv.ID
	proxy := sleepproxy.New(sleepproxy.Options{
		ServerID:     id,
		PublicPort:   srv.Port,
		InternalPort: srv.InternalPort,
	}, sleepproxy.Hooks{
		Wake:     func() error { return m.wake(id) },
		Awake:    func() bool { return m.isAwake(id) },
		Describe: func() sleepproxy.Description { return m.describe(id) },
	})

	if err := proxy.Start(); err != nil {
		return fmt.Errorf("server %s: %w", id, err)
	}

	m.mu.Lock()
	m.proxies[id] = proxy
	m.mu.Unlock()
	return nil
}

func (m *Manager) stopSleepProxy(id string) {
	m.mu.Lock()
	proxy, exists := m.proxies[id]
	delete(m.proxies, id)
	m.mu.Unlock()

	if exists {
		proxy.Close()
	}
}

// wake starts a sleeping server, ignoring a request for one that is already on
// its way up.
func (m *Manager) wake(id string) error {
	srv, err := GetServer(m.db, id)
	if err != nil {
		return err
	}

	switch srv.Status {
	case StatusRunning, StatusStarting:
		return nil
	}

	log.Printf("Waking server %s", id)
	return m.StartServer(id)
}

func (m *Manager) isAwake(id string) bool {
	srv, err := GetServer(m.db, id)
	if err != nil {
		return false
	}
	if srv.Status != StatusRunning {
		return false
	}

	// The status flips to running before the Minecraft server is listening;
	// only claim it is awake once its port answers.
	m.mu.RLock()
	_, hasProcess := m.processes[id]
	m.mu.RUnlock()
	return hasProcess
}

func (m *Manager) describe(id string) sleepproxy.Description {
	desc := sleepproxy.Description{
		VersionName:  "RealmRunner",
		MaxPlayers:   20,
		MOTD:         "§bSleeping§r - join to wake this realm",
		StartingMOTD: "§eStarting up§r - reconnect in a moment",
	}

	srv, err := GetServer(m.db, id)
	if err != nil {
		return desc
	}

	desc.VersionName = srv.Version
	desc.Starting = srv.Status == StatusStarting
	if srv.Status == StatusCrashed {
		desc.MOTD = "§cThis realm crashed§r - join to try starting it again"
	}

	if props, err := ReadProperties(m.getServerDir(id) + "/server.properties"); err == nil {
		if motd := props["motd"]; motd != "" && srv.Status != StatusCrashed {
			desc.MOTD = motd + " §7(sleeping - join to wake)"
		}
		if max := props["max-players"]; max != "" {
			fmt.Sscanf(max, "%d", &desc.MaxPlayers)
		}
	}

	return desc
}

// SetAutoSleep turns idle shutdown on or off. The server must be stopped
// because the Minecraft process has to move between the public and the internal
// port.
func (m *Manager) SetAutoSleep(id string, enabled bool, idleTimeoutMin int) error {
	srv, err := GetServer(m.db, id)
	if err != nil {
		return err
	}

	if srv.Status == StatusRunning || srv.Status == StatusStarting || srv.Status == StatusStopping {
		return fmt.Errorf("server must be stopped to change auto-sleep")
	}

	if idleTimeoutMin <= 0 {
		idleTimeoutMin = defaultIdleTimeoutMin
	}
	if err := SetAutoSleep(m.db, id, enabled, idleTimeoutMin); err != nil {
		return err
	}

	srv.AutoSleep = enabled
	srv.IdleTimeoutMin = idleTimeoutMin

	if !enabled {
		m.stopSleepProxy(id)
		if srv.Status == StatusSleeping {
			UpdateServerStatus(m.db, id, StatusStopped)
		}
		// Hand the public port back to the Minecraft process.
		_, err := m.ensureControlConfig(srv)
		return err
	}

	if err := m.ensureSleepProxy(srv); err != nil {
		SetAutoSleep(m.db, id, false, idleTimeoutMin)
		return err
	}
	if srv.Status == StatusStopped {
		UpdateServerStatus(m.db, id, StatusSleeping)
	}
	// Move the Minecraft process to the internal port for its next start.
	_, err = m.ensureControlConfig(srv)
	return err
}

// idleWatcher puts empty servers to sleep.
func (m *Manager) idleWatcher() {
	ticker := time.NewTicker(idleCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		m.checkIdleServers()
	}
}

func (m *Manager) checkIdleServers() {
	servers, err := GetAllServers(m.db)
	if err != nil {
		return
	}

	for _, srv := range servers {
		if !srv.AutoSleep || srv.Status != StatusRunning {
			m.clearIdle(srv.ID)
			continue
		}

		count, err := m.PlayerCount(srv.ID)
		if err != nil {
			// A server that cannot be asked is left alone rather than stopped.
			continue
		}
		if count > 0 {
			m.clearIdle(srv.ID)
			continue
		}

		timeout := time.Duration(srv.IdleTimeoutMin) * time.Minute
		if timeout <= 0 {
			timeout = defaultIdleTimeoutMin * time.Minute
		}

		if idleFor := m.markIdle(srv.ID); idleFor >= timeout {
			log.Printf("Server %s has been empty for %s; putting it to sleep", srv.ID, idleFor.Round(time.Second))
			if err := m.Sleep(srv.ID); err != nil {
				log.Printf("Failed to put server %s to sleep: %v", srv.ID, err)
			}
			m.clearIdle(srv.ID)
		}
	}
}

// markIdle records that a server was seen empty and returns how long it has
// been empty for.
func (m *Manager) markIdle(id string) time.Duration {
	m.sleepMu.Lock()
	defer m.sleepMu.Unlock()

	if m.sleepStates == nil {
		m.sleepStates = make(map[string]*sleepState)
	}
	state, exists := m.sleepStates[id]
	if !exists {
		m.sleepStates[id] = &sleepState{emptySince: time.Now()}
		return 0
	}
	return time.Since(state.emptySince)
}

func (m *Manager) clearIdle(id string) {
	m.sleepMu.Lock()
	defer m.sleepMu.Unlock()
	delete(m.sleepStates, id)
}

// Sleep stops a server and leaves its port held by the proxy.
func (m *Manager) Sleep(id string) error {
	srv, err := GetServer(m.db, id)
	if err != nil {
		return err
	}
	if !srv.AutoSleep {
		return fmt.Errorf("auto-sleep is not enabled for this server")
	}

	// Give anyone still connected a moment's warning.
	m.RCONExecute(id, "say This realm is going to sleep. It will start again when someone joins.")

	if err := m.StopServer(id); err != nil {
		return err
	}

	return UpdateServerStatus(m.db, id, StatusSleeping)
}

// Shutdown closes every sleep proxy. It is called when the process exits.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	proxies := make([]*sleepproxy.Proxy, 0, len(m.proxies))
	for id, proxy := range m.proxies {
		proxies = append(proxies, proxy)
		delete(m.proxies, id)
	}
	m.mu.Unlock()

	for _, proxy := range proxies {
		proxy.Close()
	}
}

// WakeServer starts a sleeping server on request from the UI.
func (m *Manager) WakeServer(id string) error {
	return m.wake(id)
}

// SetAutoRestart controls whether this server is restarted after a crash.
func (m *Manager) SetAutoRestart(id string, enabled bool) error {
	if _, err := GetServer(m.db, id); err != nil {
		return err
	}
	m.restarts.reset(id)
	return SetAutoRestart(m.db, id, enabled)
}
