package server

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/wzin/realmrunner/config"
)

type Manager struct {
	db        *sql.DB
	config    *config.Config
	processes map[string]*Process
	mu        sync.RWMutex
}

func NewManager(db *sql.DB, cfg *config.Config) *Manager {
	m := &Manager{
		db:        db,
		config:    cfg,
		processes: make(map[string]*Process),
	}

	// Clean up orphaned server statuses on startup
	m.cleanupOrphanedStatuses()

	return m
}

func (m *Manager) cleanupOrphanedStatuses() {
	// Reset all running/starting/stopping servers to stopped on startup
	// since processes don't survive container restarts
	query := `UPDATE servers SET status = ? WHERE status IN (?, ?, ?)`
	_, err := m.db.Exec(query, StatusStopped, StatusRunning, StatusStarting, StatusStopping)
	if err != nil {
		// Log but don't fail
		fmt.Printf("Failed to cleanup orphaned statuses: %v\n", err)
	}
}

func (m *Manager) CreateServer(name, version string, port int) (*Server, error) {
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

	// Start process
	serverDir := m.getServerDir(id)
	process, err := StartProcess(serverDir, server.Port, m.config.MemoryMB)
	if err != nil {
		UpdateServerStatus(m.db, id, StatusStopped)
		return fmt.Errorf("failed to start server: %w", err)
	}

	// Store process
	m.mu.Lock()
	m.processes[id] = process
	m.mu.Unlock()

	// Update status to running
	UpdateServerStatus(m.db, id, StatusRunning)
	UpdateServerLastStarted(m.db, id, time.Now())

	return nil
}

func (m *Manager) StopServer(id string) error {
	server, err := GetServer(m.db, id)
	if err != nil {
		return err
	}

	if server.Status != StatusRunning {
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
