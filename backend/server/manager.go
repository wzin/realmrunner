package server

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/wzin/realmrunner/cgroup"
	"github.com/wzin/realmrunner/config"
	"github.com/wzin/realmrunner/metrics"
	"github.com/wzin/realmrunner/minecraft"
	"github.com/wzin/realmrunner/sleepproxy"
)

type Manager struct {
	db               *sql.DB
	config           *config.Config
	processes        map[string]*Process
	collector        *metrics.Collector
	registry         *minecraft.Registry
	cgroupMgr        *cgroup.Manager
	restarts         *restartTracker
	exitDescriptions *exitDescriptions
	mu               sync.RWMutex

	// Auto-sleep: one proxy per server holding its public port, plus how long
	// each server has been empty.
	proxies     map[string]*sleepproxy.Proxy
	sleepStates map[string]*sleepState
	sleepMu     sync.Mutex
}

func NewManager(db *sql.DB, cfg *config.Config, collector *metrics.Collector, registry *minecraft.Registry, cgroupMgr *cgroup.Manager) *Manager {
	m := &Manager{
		db:               db,
		config:           cfg,
		processes:        make(map[string]*Process),
		collector:        collector,
		registry:         registry,
		cgroupMgr:        cgroupMgr,
		restarts:         newRestartTracker(),
		exitDescriptions: newExitDescriptions(),

		proxies:     make(map[string]*sleepproxy.Proxy),
		sleepStates: make(map[string]*sleepState),
	}

	// Clean up orphaned server statuses on startup
	m.cleanupOrphanedStatuses()

	// Fix ready flag for servers that have server.jar but ready=0
	m.fixReadyFlags()

	return m
}

func (m *Manager) cleanupOrphanedStatuses() {
	log.Println("Running startup cleanup for orphaned server statuses...")

	// Reset all running/starting/stopping servers to stopped on startup
	// since processes don't survive container restarts
	query := `UPDATE servers SET status = ? WHERE status IN (?, ?, ?)`
	result, err := m.db.Exec(query, StatusStopped, StatusRunning, StatusStarting, StatusStopping)
	if err != nil {
		log.Printf("ERROR: Failed to cleanup orphaned statuses: %v\n", err)
		return
	}

	rows, _ := result.RowsAffected()
	log.Printf("Startup cleanup: Reset %d orphaned server(s) to stopped status\n", rows)
}

func (m *Manager) fixReadyFlags() {
	// Collect IDs first, then update (avoid SQLite lock from open rows)
	rows, err := m.db.Query("SELECT id FROM servers WHERE ready = 0")
	if err != nil {
		return
	}

	var ids []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()

	fixed := 0
	for _, id := range ids {
		jarPath := filepath.Join(m.getServerDir(id), "server.jar")
		if _, err := os.Stat(jarPath); err == nil {
			if _, err := m.db.Exec("UPDATE servers SET ready = 1 WHERE id = ?", id); err != nil {
				log.Printf("Failed to set ready for %s: %v", id, err)
			} else {
				fixed++
			}
		}
	}
	if fixed > 0 {
		log.Printf("Fixed ready flag for %d server(s) with existing JARs", fixed)
	}
}

func (m *Manager) CreateServer(name, version, flavor string, port int) (*Server, error) {
	if flavor == "" {
		flavor = "vanilla"
	}
	// Validate port
	if !m.config.PortRange.Contains(port) {
		return nil, fmt.Errorf("port %d is outside allowed range %d-%d", port, m.config.PortRange.Min, m.config.PortRange.Max)
	}

	// Check if port is already in use
	exists, err := PortExists(m.db, port)
	if err != nil {
		return nil, fmt.Errorf("failed to check port availability: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("port %d is already in use", port)
	}

	// Create server record. The control port and password are provisioned on
	// first start, but the defaults have to be set here: the insert would
	// otherwise override the column defaults with zero values.
	server := &Server{
		ID:             uuid.New().String(),
		Name:           name,
		Version:        version,
		Flavor:         flavor,
		Port:           port,
		Status:         StatusStopped,
		CreatedAt:      time.Now(),
		InternalPort:   DerivedInternalPort(port),
		RCONPort:       DerivedRCONPort(port),
		IdleTimeoutMin: defaultIdleTimeoutMin,
		AutoRestart:    true,
	}

	if err := CreateServer(m.db, server); err != nil {
		return nil, fmt.Errorf("failed to create server: %w", err)
	}

	// Create server directory
	serverDir := m.getServerDir(server.ID)
	if err := os.MkdirAll(serverDir, 0755); err != nil {
		DeleteServer(m.db, server.ID)
		return nil, fmt.Errorf("failed to create server directory: %w", err)
	}

	return server, nil
}

func (m *Manager) GetServer(id string) (*Server, error) {
	return GetServer(m.db, id)
}

func (m *Manager) GetAllServers() ([]*Server, error) {
	return GetAllServers(m.db)
}

func (m *Manager) StartServer(id string) error {
	// Check max running limit
	runningCount, err := CountRunningServers(m.db)
	if err != nil {
		return fmt.Errorf("failed to count running servers: %w", err)
	}
	if runningCount >= m.config.MaxRunning {
		return fmt.Errorf("maximum number of running servers (%d) reached", m.config.MaxRunning)
	}

	server, err := GetServer(m.db, id)
	if err != nil {
		return err
	}

	if server.Status == StatusRunning {
		return fmt.Errorf("server is already running")
	}

	// Update status to starting
	if err := UpdateServerStatus(m.db, id, StatusStarting); err != nil {
		return err
	}

	// Get start command from provider
	// Refuse to start rather than let the JVM exit a second later with an error
	// nobody sees.
	if err := preflightJava(server.Version); err != nil {
		UpdateServerStatus(m.db, id, m.restingStatus(id))
		RecordExit(m.db, id, 0, err.Error())
		return err
	}

	// Provision the RCON control channel and write server.properties.
	listenPort, err := m.ensureControlConfig(server)
	if err != nil {
		UpdateServerStatus(m.db, id, m.restingStatus(id))
		return err
	}

	serverDir := m.getServerDir(id)
	heapMB := m.heapFor(server)
	// Minecraft versions require different Java runtimes (26.x needs Java 25),
	// so resolve the interpreter from the server's version.
	cmd := minecraft.JavaCommandForVersion(server.Version)
	args := append([]string{
		fmt.Sprintf("-Xmx%dM", heapMB),
		fmt.Sprintf("-Xms%dM", heapMB),
	}, "-jar", "server.jar", "nogui")
	if m.registry != nil {
		if provider, ok := m.registry.GetProvider(server.Flavor); ok {
			cmd, args = provider.StartCommand(serverDir, heapMB, server.Version)
		}
	}
	log.Printf("Starting server %s (%s %s) with %s", id, server.Flavor, server.Version, cmd)

	// Start process
	process, err := StartProcess(serverDir, listenPort, cmd, args)
	if err != nil {
		UpdateServerStatus(m.db, id, m.restingStatus(id))
		return fmt.Errorf("failed to start server: %w", err)
	}

	// Store process
	m.mu.Lock()
	m.processes[id] = process
	m.mu.Unlock()

	// Monitor process in background and update status if it dies
	go m.monitorProcess(id, process)

	// Update status to running
	UpdateServerStatus(m.db, id, StatusRunning)
	UpdateServerLastStarted(m.db, id, time.Now())
	ClearLastError(m.db, id)

	// Apply cgroup limits if configured
	if m.cgroupMgr != nil && (server.CPULimit > 0 || server.MemoryLimitMB > 0) {
		if err := m.cgroupMgr.CreateCgroup(id, server.CPULimit, server.MemoryLimitMB); err != nil {
			log.Printf("Failed to create cgroup for %s: %v", id, err)
		} else if err := m.cgroupMgr.AssignProcess(id, process.PID()); err != nil {
			log.Printf("Failed to assign process to cgroup for %s: %v", id, err)
		}
	}

	// Start metrics collection
	if m.collector != nil {
		m.collector.StartCollecting(id, process.PID(), server.Port)
	}

	return nil
}

func (m *Manager) monitorProcess(id string, process *Process) {
	// Wait for process to exit
	if process.cmd != nil && process.cmd.Process != nil {
		waitErr := process.cmd.Wait()
		exitCode := exitCodeOf(waitErr)
		uptime := process.Uptime()
		m.exitDescriptions.set(id, exitDescription(waitErr, exitCode))

		// Stop metrics collection
		if m.collector != nil {
			m.collector.StopCollecting(id)
		}

		// Remove cgroup
		if m.cgroupMgr != nil {
			m.cgroupMgr.RemoveCgroup(id)
		}

		// Process exited - update status
		m.mu.Lock()
		delete(m.processes, id)
		m.mu.Unlock()

		if process.StopRequested() {
			// With auto-sleep the proxy keeps holding the port, so the server
			// is asleep rather than stopped.
			status := m.restingStatus(id)
			UpdateServerStatus(m.db, id, status)
			log.Printf("Server %s process exited, status set to %s", id, status)
			m.restarts.reset(id)
			return
		}

		// A server that stayed up for a while has earned a clean slate.
		if uptime >= stableUptime {
			m.restarts.reset(id)
		}

		m.handleCrash(id, exitCode)
	}
}

func (m *Manager) StopServer(id string) error {
	server, err := GetServer(m.db, id)
	if err != nil {
		return err
	}

	if server.Status != StatusRunning && server.Status != StatusStarting {
		return fmt.Errorf("server is not running")
	}

	// Update status to stopping
	if err := UpdateServerStatus(m.db, id, StatusStopping); err != nil {
		return err
	}

	// Get process
	m.mu.RLock()
	process, exists := m.processes[id]
	m.mu.RUnlock()

	if exists {
		// Stop metrics collection
		if m.collector != nil {
			m.collector.StopCollecting(id)
		}

		// Remove cgroup
		if m.cgroupMgr != nil {
			m.cgroupMgr.RemoveCgroup(id)
		}

		// Stop process
		if err := process.Stop(); err != nil {
			return fmt.Errorf("failed to stop server: %w", err)
		}

		// Remove process from map
		m.mu.Lock()
		delete(m.processes, id)
		m.mu.Unlock()
	}

	// With auto-sleep the port stays held by the proxy, so the server is
	// asleep rather than simply stopped.
	UpdateServerStatus(m.db, id, m.restingStatus(id))

	return nil
}

// restingStatus is the status a server takes when it is not running: asleep if
// its port is still held for players, otherwise stopped.
func (m *Manager) restingStatus(id string) string {
	srv, err := GetServer(m.db, id)
	if err == nil && srv.AutoSleep {
		return StatusSleeping
	}
	return StatusStopped
}

func (m *Manager) ForceStopServer(id string) error {
	server, err := GetServer(m.db, id)
	if err != nil {
		return err
	}

	if server.Status != StatusRunning && server.Status != StatusStarting && server.Status != StatusStopping {
		return fmt.Errorf("server is not running")
	}

	m.mu.RLock()
	process, exists := m.processes[id]
	m.mu.RUnlock()

	if exists {
		if m.collector != nil {
			m.collector.StopCollecting(id)
		}
		if m.cgroupMgr != nil {
			m.cgroupMgr.RemoveCgroup(id)
		}

		process.ForceKill()

		m.mu.Lock()
		delete(m.processes, id)
		m.mu.Unlock()
	}

	UpdateServerStatus(m.db, id, m.restingStatus(id))
	return nil
}

func (m *Manager) ResetServer(id string) error {
	server, err := GetServer(m.db, id)
	if err != nil {
		return err
	}

	if server.Status == StatusRunning {
		return fmt.Errorf("cannot reset a running server, stop it first")
	}

	// Delete world directories
	serverDir := m.getServerDir(id)
	worldDirs := []string{"world", "world_nether", "world_the_end"}

	for _, worldDir := range worldDirs {
		worldPath := filepath.Join(serverDir, worldDir)
		if _, err := os.Stat(worldPath); err == nil {
			if err := os.RemoveAll(worldPath); err != nil {
				return fmt.Errorf("failed to delete %s: %w", worldDir, err)
			}
			log.Printf("Deleted %s for server %s", worldDir, id)
		}
	}

	return nil
}

func (m *Manager) WipeoutServer(id string) error {
	server, err := GetServer(m.db, id)
	if err != nil {
		return err
	}

	if server.Status == StatusRunning {
		return fmt.Errorf("cannot wipeout a running server, stop it first")
	}

	// Delete server directory
	serverDir := m.getServerDir(id)
	if err := os.RemoveAll(serverDir); err != nil {
		return fmt.Errorf("failed to delete server directory: %w", err)
	}

	// Delete from database
	if err := DeleteServer(m.db, id); err != nil {
		return fmt.Errorf("failed to delete server from database: %w", err)
	}

	return nil
}

func (m *Manager) SendCommand(id, command string) error {
	// RCON is the reliable path: it confirms delivery and works even for a
	// server this process did not start.
	if _, err := m.RCONExecute(id, command); err == nil {
		return nil
	}

	m.mu.RLock()
	process, exists := m.processes[id]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("server is not running")
	}

	return process.SendCommand(command)
}

func (m *Manager) GetProcess(id string) (*Process, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	process, exists := m.processes[id]
	return process, exists
}

func (m *Manager) getServerDir(id string) string {
	return filepath.Join(m.config.DataDir, "servers", id)
}

func (m *Manager) GetServerDir(id string) string {
	return m.getServerDir(id)
}

func (m *Manager) SetLimits(id string, cpuLimit float64, memoryLimitMB int) error {
	srv, err := GetServer(m.db, id)
	if err != nil {
		return err
	}
	if srv.Status != StatusStopped && srv.Status != StatusSleeping && srv.Status != StatusCrashed {
		return fmt.Errorf("server must be stopped to change limits")
	}
	return UpdateServerLimits(m.db, id, cpuLimit, memoryLimitMB)
}

// heapFor is the Java heap a server should run with: its own setting, or the
// instance-wide default when it has none.
func (m *Manager) heapFor(srv *Server) int {
	if srv.HeapMB > 0 {
		return srv.HeapMB
	}
	return m.config.MemoryMB
}

// SetHeapMB changes one server's Java heap. It takes effect on the next start.
func (m *Manager) SetHeapMB(id string, heapMB int) error {
	srv, err := GetServer(m.db, id)
	if err != nil {
		return err
	}
	if heapMB < 0 {
		return fmt.Errorf("heap size cannot be negative")
	}
	if heapMB > 0 && heapMB < 512 {
		return fmt.Errorf("a Minecraft server needs at least 512 MB of heap")
	}
	if srv.MemoryLimitMB > 0 && heapMB > srv.MemoryLimitMB {
		return fmt.Errorf("the heap (%d MB) cannot be larger than the server's memory limit (%d MB)", heapMB, srv.MemoryLimitMB)
	}
	return SetHeapMB(m.db, id, heapMB)
}

func (m *Manager) SetRestartSchedule(id, schedule string) error {
	return UpdateRestartSchedule(m.db, id, schedule)
}

func (m *Manager) GetRegistry() *minecraft.Registry {
	return m.registry
}

func (m *Manager) GetCollector() *metrics.Collector {
	return m.collector
}

func (m *Manager) GetDB() *sql.DB {
	return m.db
}
