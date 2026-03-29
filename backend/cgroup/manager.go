package cgroup

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const cgroupBase = "/sys/fs/cgroup/realmrunner"

type Manager struct {
	available bool
}

func NewManager() *Manager {
	m := &Manager{}
	m.available = m.init()
	if m.available {
		log.Println("Cgroup v2 support enabled")
	} else {
		log.Println("Cgroup v2 not available, resource limits disabled")
	}
	return m
}

func (m *Manager) Available() bool {
	return m.available
}

func (m *Manager) init() bool {
	// Check if cgroups v2 is available
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err != nil {
		return false
	}

	// Create parent cgroup
	if err := os.MkdirAll(cgroupBase, 0755); err != nil {
		log.Printf("Cannot create cgroup directory: %v", err)
		return false
	}

	// Enable cpu and memory controllers
	parentDir := filepath.Dir(cgroupBase)
	controlFile := filepath.Join(parentDir, "cgroup.subtree_control")
	data, err := os.ReadFile(controlFile)
	if err != nil {
		return false
	}

	controllers := string(data)
	needEnable := []string{}
	if !strings.Contains(controllers, "cpu") {
		needEnable = append(needEnable, "+cpu")
	}
	if !strings.Contains(controllers, "memory") {
		needEnable = append(needEnable, "+memory")
	}

	if len(needEnable) > 0 {
		if err := os.WriteFile(controlFile, []byte(strings.Join(needEnable, " ")), 0644); err != nil {
			log.Printf("Cannot enable cgroup controllers: %v", err)
			// Still try to proceed - controllers might already be available
		}
	}

	return true
}

func (m *Manager) CreateCgroup(serverID string, cpuLimit float64, memoryLimitMB int) error {
	if !m.available {
		return nil
	}

	dir := filepath.Join(cgroupBase, serverID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create cgroup: %w", err)
	}

	// Set CPU limit: cpuLimit is number of cores (e.g., 1.5 = 150% of one core)
	if cpuLimit > 0 {
		quota := int(cpuLimit * 100000) // period is 100000us
		cpuMax := fmt.Sprintf("%d 100000", quota)
		if err := os.WriteFile(filepath.Join(dir, "cpu.max"), []byte(cpuMax), 0644); err != nil {
			log.Printf("Failed to set cpu.max for %s: %v", serverID, err)
		}
	}

	// Set memory limit
	if memoryLimitMB > 0 {
		memBytes := int64(memoryLimitMB) * 1024 * 1024
		if err := os.WriteFile(filepath.Join(dir, "memory.max"), []byte(strconv.FormatInt(memBytes, 10)), 0644); err != nil {
			log.Printf("Failed to set memory.max for %s: %v", serverID, err)
		}
		// Disable swap
		os.WriteFile(filepath.Join(dir, "memory.swap.max"), []byte("0"), 0644)
	}

	return nil
}

func (m *Manager) AssignProcess(serverID string, pid int) error {
	if !m.available {
		return nil
	}

	procsFile := filepath.Join(cgroupBase, serverID, "cgroup.procs")
	return os.WriteFile(procsFile, []byte(strconv.Itoa(pid)), 0644)
}

func (m *Manager) RemoveCgroup(serverID string) error {
	if !m.available {
		return nil
	}

	dir := filepath.Join(cgroupBase, serverID)
	// cgroup dir can only be removed when empty (no processes)
	return os.Remove(dir)
}
