package metrics

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type Metric struct {
	ID          int64     `json:"id"`
	ServerID    string    `json:"server_id"`
	Timestamp   time.Time `json:"timestamp"`
	CPUPercent  float64   `json:"cpu_percent"`
	MemoryMB    float64   `json:"memory_mb"`
	PlayerCount int       `json:"player_count"`
	PlayerNames []string  `json:"player_names"`
}

type MetricPoint struct {
	Timestamp   time.Time `json:"timestamp"`
	CPUPercent  float64   `json:"cpu_percent"`
	MemoryMB    float64   `json:"memory_mb"`
	PlayerCount int       `json:"player_count"`
}

func InitMetricsTable(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		server_id TEXT NOT NULL,
		timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		cpu_percent REAL,
		memory_mb REAL,
		player_count INTEGER DEFAULT 0,
		player_names TEXT DEFAULT '[]'
	);
	CREATE INDEX IF NOT EXISTS idx_metrics_server_time ON metrics(server_id, timestamp);
	`
	_, err := db.Exec(schema)
	return err
}

func InsertMetric(db *sql.DB, m *Metric) error {
	names, _ := json.Marshal(m.PlayerNames)
	query := `INSERT INTO metrics (server_id, timestamp, cpu_percent, memory_mb, player_count, player_names) VALUES (?, ?, ?, ?, ?, ?)`
	_, err := db.Exec(query, m.ServerID, m.Timestamp, m.CPUPercent, m.MemoryMB, m.PlayerCount, string(names))
	return err
}

func GetLatestMetric(db *sql.DB, serverID string) (*Metric, error) {
	query := `SELECT id, server_id, timestamp, cpu_percent, memory_mb, player_count, player_names FROM metrics WHERE server_id = ? ORDER BY timestamp DESC LIMIT 1`
	m := &Metric{}
	var namesStr string
	err := db.QueryRow(query, serverID).Scan(&m.ID, &m.ServerID, &m.Timestamp, &m.CPUPercent, &m.MemoryMB, &m.PlayerCount, &namesStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(namesStr), &m.PlayerNames)
	return m, nil
}

func GetMetricsHistory(db *sql.DB, serverID string, rangeStr string) ([]MetricPoint, error) {
	var since time.Time
	var bucket string

	now := time.Now()
	switch rangeStr {
	case "1h":
		since = now.Add(-1 * time.Hour)
		bucket = "" // raw data
	case "24h":
		since = now.Add(-24 * time.Hour)
		bucket = "%Y-%m-%d %H:%M" // truncate to 5-min by grouping
	case "7d":
		since = now.Add(-7 * 24 * time.Hour)
		bucket = "%Y-%m-%d %H" // hourly
	case "30d":
		since = now.Add(-30 * 24 * time.Hour)
		bucket = "%Y-%m-%d %H" // hourly (2h would need custom logic)
	default:
		since = now.Add(-24 * time.Hour)
		bucket = "%Y-%m-%d %H:%M"
	}

	var query string
	if bucket == "" {
		query = `SELECT timestamp, cpu_percent, memory_mb, player_count FROM metrics WHERE server_id = ? AND timestamp > ? ORDER BY timestamp`
	} else {
		// For 5-min bucketing on 24h: group by truncated minute / 5
		if rangeStr == "24h" {
			query = fmt.Sprintf(`SELECT MAX(timestamp), AVG(cpu_percent), AVG(memory_mb), MAX(player_count) FROM metrics WHERE server_id = ? AND timestamp > ? GROUP BY strftime('%s', timestamp), CAST(strftime('%%M', timestamp) AS INTEGER) / 5 ORDER BY MAX(timestamp)`, bucket)
		} else {
			query = fmt.Sprintf(`SELECT MAX(timestamp), AVG(cpu_percent), AVG(memory_mb), MAX(player_count) FROM metrics WHERE server_id = ? AND timestamp > ? GROUP BY strftime('%s', timestamp) ORDER BY MAX(timestamp)`, bucket)
		}
	}

	rows, err := db.Query(query, serverID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var p MetricPoint
		if err := rows.Scan(&p.Timestamp, &p.CPUPercent, &p.MemoryMB, &p.PlayerCount); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, nil
}

func PurgeOldMetrics(db *sql.DB, olderThan time.Time) (int64, error) {
	result, err := db.Exec(`DELETE FROM metrics WHERE timestamp < ?`, olderThan)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
