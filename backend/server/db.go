package server

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Server struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Version       string     `json:"version"`
	Flavor        string     `json:"flavor"`
	Port          int        `json:"port"`
	Status        string     `json:"status"`
	CPULimit        float64    `json:"cpu_limit"`
	MemoryLimitMB   int        `json:"memory_limit_mb"`
	RestartSchedule string     `json:"restart_schedule"`
	Ready           bool       `json:"ready"`
	CreatedAt     time.Time  `json:"created_at"`
	LastStartedAt *time.Time `json:"last_started_at,omitempty"`
}

const (
	StatusStopped  = "stopped"
	StatusStarting = "starting"
	StatusRunning  = "running"
	StatusStopping = "stopping"
)

func InitDB(dataDir string) (*sql.DB, error) {
	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	dbPath := filepath.Join(dataDir, "realmrunner.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create servers table
	schema := `
	CREATE TABLE IF NOT EXISTS servers (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		version TEXT NOT NULL,
		port INTEGER NOT NULL UNIQUE,
		status TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_started_at TIMESTAMP
	);
	`

	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	// Migrations
	db.Exec("ALTER TABLE servers ADD COLUMN flavor TEXT NOT NULL DEFAULT 'vanilla'")
	db.Exec("ALTER TABLE servers ADD COLUMN cpu_limit REAL DEFAULT 0")
	db.Exec("ALTER TABLE servers ADD COLUMN memory_limit_mb INTEGER DEFAULT 0")
	db.Exec("ALTER TABLE servers ADD COLUMN restart_schedule TEXT DEFAULT ''")
	db.Exec("ALTER TABLE servers ADD COLUMN ready INTEGER DEFAULT 0")

	return db, nil
}

func CreateServer(db *sql.DB, server *Server) error {
	query := `
	INSERT INTO servers (id, name, version, flavor, port, status, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err := db.Exec(query, server.ID, server.Name, server.Version, server.Flavor, server.Port, server.Status, server.CreatedAt)
	return err
}

func GetServer(db *sql.DB, id string) (*Server, error) {
	query := `SELECT id, name, version, flavor, port, status, cpu_limit, memory_limit_mb, restart_schedule, ready, created_at, last_started_at FROM servers WHERE id = ?`
	server := &Server{}
	err := db.QueryRow(query, id).Scan(
		&server.ID,
		&server.Name,
		&server.Version,
		&server.Flavor,
		&server.Port,
		&server.Status,
		&server.CPULimit,
		&server.MemoryLimitMB,
		&server.RestartSchedule,
		&server.Ready,
		&server.CreatedAt,
		&server.LastStartedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("server not found")
	}
	return server, err
}

func GetAllServers(db *sql.DB) ([]*Server, error) {
	query := `SELECT id, name, version, flavor, port, status, cpu_limit, memory_limit_mb, restart_schedule, ready, created_at, last_started_at FROM servers ORDER BY created_at DESC`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	servers := []*Server{}
	for rows.Next() {
		server := &Server{}
		err := rows.Scan(
			&server.ID,
			&server.Name,
			&server.Version,
			&server.Flavor,
			&server.Port,
			&server.Status,
			&server.CPULimit,
			&server.MemoryLimitMB,
			&server.RestartSchedule,
			&server.Ready,
			&server.CreatedAt,
			&server.LastStartedAt,
		)
		if err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}

	return servers, nil
}

func UpdateServerStatus(db *sql.DB, id string, status string) error {
	query := `UPDATE servers SET status = ? WHERE id = ?`
	_, err := db.Exec(query, status, id)
	return err
}

func UpdateServerLastStarted(db *sql.DB, id string, timestamp time.Time) error {
	query := `UPDATE servers SET last_started_at = ? WHERE id = ?`
	_, err := db.Exec(query, timestamp, id)
	return err
}

func SetServerReady(db *sql.DB, id string, ready bool) error {
	val := 0
	if ready {
		val = 1
	}
	_, err := db.Exec("UPDATE servers SET ready = ? WHERE id = ?", val, id)
	return err
}

func UpdateRestartSchedule(db *sql.DB, id, schedule string) error {
	query := `UPDATE servers SET restart_schedule = ? WHERE id = ?`
	_, err := db.Exec(query, schedule, id)
	return err
}

func UpdateServerLimits(db *sql.DB, id string, cpuLimit float64, memoryLimitMB int) error {
	query := `UPDATE servers SET cpu_limit = ?, memory_limit_mb = ? WHERE id = ?`
	_, err := db.Exec(query, cpuLimit, memoryLimitMB, id)
	return err
}

func UpdateServerVersion(db *sql.DB, id, version, flavor string) error {
	query := `UPDATE servers SET version = ?, flavor = ? WHERE id = ?`
	_, err := db.Exec(query, version, flavor, id)
	return err
}

func DeleteServer(db *sql.DB, id string) error {
	query := `DELETE FROM servers WHERE id = ?`
	_, err := db.Exec(query, id)
	return err
}

func PortExists(db *sql.DB, port int) (bool, error) {
	query := `SELECT COUNT(*) FROM servers WHERE port = ?`
	var count int
	err := db.QueryRow(query, port).Scan(&count)
	return count > 0, err
}

func CountRunningServers(db *sql.DB) (int, error) {
	query := `SELECT COUNT(*) FROM servers WHERE status = ?`
	var count int
	err := db.QueryRow(query, StatusRunning).Scan(&count)
	return count, err
}
