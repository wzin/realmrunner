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
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Version         string     `json:"version"`
	Flavor          string     `json:"flavor"`
	Port            int        `json:"port"`
	Status          string     `json:"status"`
	CPULimit        float64    `json:"cpu_limit"`
	MemoryLimitMB   int        `json:"memory_limit_mb"`
	RestartSchedule string     `json:"restart_schedule"`
	Ready           bool       `json:"ready"`
	ShareToken      string     `json:"share_token,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	LastStartedAt   *time.Time `json:"last_started_at,omitempty"`

	// RCON control channel. The port is bound on loopback only and is never
	// published by the container.
	RCONPort     int    `json:"rcon_port"`
	RCONPassword string `json:"-"`

	// Auto-sleep. InternalPort is where the Minecraft server actually listens
	// when the port is fronted by RealmRunner's proxy; Port stays the address
	// players connect to.
	AutoSleep      bool `json:"auto_sleep"`
	IdleTimeoutMin int  `json:"idle_timeout_min"`
	InternalPort   int  `json:"internal_port"`

	// Crash handling.
	AutoRestart  bool   `json:"auto_restart"`
	LastExitCode int    `json:"last_exit_code"`
	LastError    string `json:"last_error,omitempty"`
}

const (
	StatusStopped  = "stopped"
	StatusStarting = "starting"
	StatusRunning  = "running"
	StatusStopping = "stopping"
	// StatusSleeping means the server is stopped but its port is held by the
	// proxy, which will start it when a player connects.
	StatusSleeping = "sleeping"
	// StatusCrashed means the process exited on its own without being asked to.
	StatusCrashed = "crashed"
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
	db.Exec("ALTER TABLE servers ADD COLUMN share_token TEXT DEFAULT ''")
	db.Exec("ALTER TABLE servers ADD COLUMN rcon_port INTEGER DEFAULT 0")
	db.Exec("ALTER TABLE servers ADD COLUMN rcon_password TEXT DEFAULT ''")
	db.Exec("ALTER TABLE servers ADD COLUMN auto_sleep INTEGER DEFAULT 0")
	db.Exec("ALTER TABLE servers ADD COLUMN idle_timeout_min INTEGER DEFAULT 15")
	db.Exec("ALTER TABLE servers ADD COLUMN internal_port INTEGER DEFAULT 0")
	db.Exec("ALTER TABLE servers ADD COLUMN auto_restart INTEGER DEFAULT 1")
	db.Exec("ALTER TABLE servers ADD COLUMN last_exit_code INTEGER DEFAULT 0")
	db.Exec("ALTER TABLE servers ADD COLUMN last_error TEXT DEFAULT ''")

	return db, nil
}

func CreateServer(db *sql.DB, server *Server) error {
	query := `
	INSERT INTO servers (id, name, version, flavor, port, status, created_at,
	                     rcon_port, rcon_password, internal_port, idle_timeout_min, auto_restart)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := db.Exec(query, server.ID, server.Name, server.Version, server.Flavor, server.Port,
		server.Status, server.CreatedAt, server.RCONPort, server.RCONPassword, server.InternalPort,
		server.IdleTimeoutMin, server.AutoRestart)
	return err
}

func GetServer(db *sql.DB, id string) (*Server, error) {
	query := `SELECT id, name, version, flavor, port, status, cpu_limit, memory_limit_mb, restart_schedule, ready, share_token, created_at, last_started_at, rcon_port, rcon_password, auto_sleep, idle_timeout_min, internal_port, auto_restart, last_exit_code, last_error FROM servers WHERE id = ?`
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
		&server.ShareToken,
		&server.CreatedAt,
		&server.LastStartedAt,
		&server.RCONPort,
		&server.RCONPassword,
		&server.AutoSleep,
		&server.IdleTimeoutMin,
		&server.InternalPort,
		&server.AutoRestart,
		&server.LastExitCode,
		&server.LastError,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("server not found")
	}
	return server, err
}

func GetAllServers(db *sql.DB) ([]*Server, error) {
	query := `SELECT id, name, version, flavor, port, status, cpu_limit, memory_limit_mb, restart_schedule, ready, share_token, created_at, last_started_at, rcon_port, rcon_password, auto_sleep, idle_timeout_min, internal_port, auto_restart, last_exit_code, last_error FROM servers ORDER BY created_at DESC`
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
			&server.ShareToken,
			&server.CreatedAt,
			&server.LastStartedAt,
			&server.RCONPort,
			&server.RCONPassword,
			&server.AutoSleep,
			&server.IdleTimeoutMin,
			&server.InternalPort,
			&server.AutoRestart,
			&server.LastExitCode,
			&server.LastError,
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

func SetShareToken(db *sql.DB, id, token string) error {
	_, err := db.Exec("UPDATE servers SET share_token = ? WHERE id = ?", token, id)
	return err
}

func GetServerByShareToken(db *sql.DB, token string) (*Server, error) {
	if token == "" {
		return nil, fmt.Errorf("empty token")
	}
	// Reuse GetServer logic but query by share_token
	query := `SELECT id, name, version, flavor, port, status, cpu_limit, memory_limit_mb, restart_schedule, ready, share_token, created_at, last_started_at FROM servers WHERE share_token = ?`
	server := &Server{}
	err := db.QueryRow(query, token).Scan(
		&server.ID, &server.Name, &server.Version, &server.Flavor,
		&server.Port, &server.Status, &server.CPULimit, &server.MemoryLimitMB,
		&server.RestartSchedule, &server.Ready, &server.ShareToken,
		&server.CreatedAt, &server.LastStartedAt,
	)
	if err != nil {
		return nil, err
	}
	return server, nil
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

// SetRCONCredentials stores the control-channel port and password for a server.
func SetRCONCredentials(db *sql.DB, id string, port int, password string) error {
	_, err := db.Exec("UPDATE servers SET rcon_port = ?, rcon_password = ? WHERE id = ?", port, password, id)
	return err
}

// SetInternalPort records the port the Minecraft process itself listens on,
// which differs from the advertised port while the proxy fronts it.
func SetInternalPort(db *sql.DB, id string, port int) error {
	_, err := db.Exec("UPDATE servers SET internal_port = ? WHERE id = ?", port, id)
	return err
}

// SetAutoSleep enables or disables idle shutdown and sets the idle timeout.
func SetAutoSleep(db *sql.DB, id string, enabled bool, idleTimeoutMin int) error {
	if idleTimeoutMin <= 0 {
		idleTimeoutMin = 15
	}
	_, err := db.Exec("UPDATE servers SET auto_sleep = ?, idle_timeout_min = ? WHERE id = ?", enabled, idleTimeoutMin, id)
	return err
}

// SetAutoRestart controls whether a crashed server is restarted automatically.
func SetAutoRestart(db *sql.DB, id string, enabled bool) error {
	_, err := db.Exec("UPDATE servers SET auto_restart = ? WHERE id = ?", enabled, id)
	return err
}

// RecordExit stores how a server process ended, so the UI can explain a crash
// instead of silently showing "stopped".
func RecordExit(db *sql.DB, id string, exitCode int, message string) error {
	_, err := db.Exec("UPDATE servers SET last_exit_code = ?, last_error = ? WHERE id = ?", exitCode, message, id)
	return err
}

// ClearLastError resets the recorded failure, used when a server starts cleanly.
func ClearLastError(db *sql.DB, id string) error {
	_, err := db.Exec("UPDATE servers SET last_exit_code = 0, last_error = '' WHERE id = ?", id)
	return err
}

// InternalPortExists reports whether another server already uses an internal port.
func InternalPortExists(db *sql.DB, port int) (bool, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM servers WHERE internal_port = ? OR port = ? OR rcon_port = ?", port, port, port).Scan(&count)
	return count > 0, err
}
