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
)

type Manager struct {
	db        *sql.DB
	config    *config.Config
	processes map[string]*Process
	collector *metrics.Collector
	registry  *minecraft.Registry
	cgroupMgr *cgroup.Manager
	mu        sync.RWMutex
}

func NewManager(db *sql.DB, cfg *config.Config, collector *metrics.Collector, registry *minecraft.Registry, cgroupMgr *cgroup.Manager) *Manager {
	m := &Manager{
		db:        db,
		config:    cfg,
		processes: make(map[string]*Process),
		collector: collector,
		registry:  registry,
		cgroupMgr: cgroupMgr,
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
	rows, err := m.db.Query("SELECT id FROM servers WHERE ready = 0")
	if err != nil {
		return
	}
	defer rows.Close()

	fixed := 0
	for rows.Next() {
		var id string
		rows.Scan(&id)
		jarPath := filepath.Join(m.getServerDir(id), "server.jar")
		if _, err := os.Stat(jarPath); err == nil {
			SetServerReady(m.db, id, true)
			fixed++
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

	// Create server record
	server := &Server{
		ID:        uuid.New().String(),
		Name:      name,
		Version:   version,
		Flavor:    flavor,
		Port:      port,
		Status:    StatusStopped,
		CreatedAt: time.Now(),
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
	serverDir := m.getServerDir(id)
	cmd := "java"
	args := []string{
		fmt.Sprintf("-Xmx%dM", m.config.MemoryMB),
		fmt.Sprintf("-Xms%dM", m.config.MemoryMB),
		"-jar", "server.jar", "nogui",
	}
	if m.registry != nil {
		if provider, ok := m.registry.GetProvider(server.Flavor); ok {
			cmd, args = provider.StartCommand(serverDir, m.config.MemoryMB)
		}
	}

	// Start process
	process, err := StartProcess(serverDir, server.Port, cmd, args)
	if err != nil {
		UpdateServerStatus(m.db, id, StatusStopped)
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
		process.cmd.Wait()

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

		UpdateServerStatus(m.db, id, StatusStopped)
		log.Printf("Server %s process exited, status set to stopped", id)
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

	// Update status to stopped
	UpdateServerStatus(m.db, id, StatusStopped)

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
	if srv.Status != StatusStopped {
		return fmt.Errorf("server must be stopped to change limits")
	}
	return UpdateServerLimits(m.db, id, cpuLimit, memoryLimitMB)
}

func (m *Manager) SetRestartSchedule(id, schedule string) error {
	return UpdateRestartSchedule(m.db, id, schedule)
}

func (m *Manager) GetRegistry() *minecraft.Registry {
	return m.registry
}

func (m *Manager) UpgradeServer(id, version, flavor string) error {
	srv, err := GetServer(m.db, id)
	if err != nil {
		return err
	}

	if srv.Status != StatusStopped {
		return fmt.Errorf("server must be stopped to upgrade")
	}

	if flavor == "" {
		flavor = srv.Flavor
	}

	// Download new server jar
	provider, ok := m.registry.GetProvider(flavor)
	if !ok {
		return fmt.Errorf("unknown server flavor: %s", flavor)
	}

	serverDir := m.getServerDir(id)

	// Remove old server.jar
	jarPath := filepath.Join(serverDir, "server.jar")
	os.Remove(jarPath)

	// Download new version
	if err := provider.DownloadServer(serverDir, version); err != nil {
		return fmt.Errorf("failed to download server: %w", err)
	}

	// Update DB
	return UpdateServerVersion(m.db, id, version, flavor)
}

func (m *Manager) GetCollector() *metrics.Collector {
	return m.collector
}

func (m *Manager) GetDB() *sql.DB {
	return m.db
}
