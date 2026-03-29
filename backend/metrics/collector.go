package metrics

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type LatestMetrics struct {
	CPUPercent  float64  `json:"cpu_percent"`
	MemoryMB    float64  `json:"memory_mb"`
	PlayerCount int      `json:"player_count"`
	PlayerNames []string `json:"player_names"`
}

type cpuSample struct {
	ticks int64
	time  time.Time
}

type Collector struct {
	db      *sql.DB
	cancels map[string]context.CancelFunc
	latest  map[string]*LatestMetrics
	mu      sync.RWMutex
}

func NewCollector(db *sql.DB) *Collector {
	c := &Collector{
		db:      db,
		cancels: make(map[string]context.CancelFunc),
		latest:  make(map[string]*LatestMetrics),
	}
	// Start daily purge
	go c.purgeLoop()
	return c
}

func (c *Collector) StartCollecting(serverID string, pid int, port int) {
	c.mu.Lock()
	// Stop existing collector if any
	if cancel, ok := c.cancels[serverID]; ok {
		cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancels[serverID] = cancel
	c.mu.Unlock()

	go c.collectLoop(ctx, serverID, pid, port)
}

func (c *Collector) StopCollecting(serverID string) {
	c.mu.Lock()
	if cancel, ok := c.cancels[serverID]; ok {
		cancel()
		delete(c.cancels, serverID)
	}
	delete(c.latest, serverID)
	c.mu.Unlock()
}

func (c *Collector) GetLatest(serverID string) *LatestMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latest[serverID]
}

func (c *Collector) collectLoop(ctx context.Context, serverID string, pid int, port int) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	var prevCPU *cpuSample

	// Collect immediately on start
	prevCPU = c.collect(serverID, pid, port, prevCPU)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			prevCPU = c.collect(serverID, pid, port, prevCPU)
		}
	}
}

func (c *Collector) collect(serverID string, pid int, port int, prevCPU *cpuSample) *cpuSample {
	// Check if process still exists
	if !processExists(pid) {
		return prevCPU
	}

	// Read CPU
	cpuTicks := readCPUTicks(pid)
	now := time.Now()
	var cpuPercent float64
	currentSample := &cpuSample{ticks: cpuTicks, time: now}

	if prevCPU != nil && cpuTicks > 0 {
		elapsed := now.Sub(prevCPU.time).Seconds()
		if elapsed > 0 {
			deltaTicks := cpuTicks - prevCPU.ticks
			// CLK_TCK is 100 on Linux
			cpuPercent = float64(deltaTicks) / (100.0 * elapsed) * 100.0
			if cpuPercent < 0 {
				cpuPercent = 0
			}
		}
	}

	// Read memory
	memoryMB := readMemoryMB(pid)

	// Query player count via MC ping
	var playerCount int
	var playerNames []string
	ping, err := QueryServerStatus(port)
	if err == nil && ping != nil {
		playerCount = ping.OnlinePlayers
		playerNames = ping.PlayerNames
	}

	// Update latest cache
	latest := &LatestMetrics{
		CPUPercent:  cpuPercent,
		MemoryMB:    memoryMB,
		PlayerCount: playerCount,
		PlayerNames: playerNames,
	}
	c.mu.Lock()
	c.latest[serverID] = latest
	c.mu.Unlock()

	// Store in DB
	metric := &Metric{
		ServerID:    serverID,
		Timestamp:   now,
		CPUPercent:  cpuPercent,
		MemoryMB:    memoryMB,
		PlayerCount: playerCount,
		PlayerNames: playerNames,
	}
	if err := InsertMetric(c.db, metric); err != nil {
		log.Printf("Failed to insert metric for %s: %v", serverID, err)
	}

	return currentSample
}

func (c *Collector) purgeLoop() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		cutoff := time.Now().Add(-30 * 24 * time.Hour)
		rows, err := PurgeOldMetrics(c.db, cutoff)
		if err != nil {
			log.Printf("Failed to purge old metrics: %v", err)
		} else if rows > 0 {
			log.Printf("Purged %d old metric rows", rows)
		}
	}
}

func processExists(pid int) bool {
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return err == nil
}

func readCPUTicks(pid int) int64 {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0
	}

	// /proc/pid/stat format: pid (comm) state ... field14=utime field15=stime
	// Find the closing paren to skip the comm field (may contain spaces)
	str := string(data)
	idx := strings.LastIndex(str, ")")
	if idx < 0 {
		return 0
	}
	fields := strings.Fields(str[idx+2:]) // skip ") "
	// fields[0] = state, fields[11] = utime (field 14), fields[12] = stime (field 15)
	if len(fields) < 13 {
		return 0
	}
	utime, _ := strconv.ParseInt(fields[11], 10, 64)
	stime, _ := strconv.ParseInt(fields[12], 10, 64)
	return utime + stime
}

func readMemoryMB(pid int) float64 {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				kb, _ := strconv.ParseFloat(parts[1], 64)
				return kb / 1024.0
			}
		}
	}
	return 0
}
