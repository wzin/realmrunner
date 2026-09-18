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
	// groupBy buckets samples so a long window stays a readable number of
	// points; an empty value means raw samples.
	var groupBy string

	now := time.Now()
	switch rangeStr {
	case "1h":
		since = now.Add(-1 * time.Hour)
		groupBy = ""
	case "24h":
		since = now.Add(-24 * time.Hour)
		groupBy = fiveMinuteBucket
	case "7d":
		since = now.Add(-7 * 24 * time.Hour)
		groupBy = hourBucket
	case "30d":
		since = now.Add(-30 * 24 * time.Hour)
		groupBy = fourHourBucket
	default:
		since = now.Add(-24 * time.Hour)
		groupBy = fiveMinuteBucket
	}

	query := `SELECT timestamp, cpu_percent, memory_mb, player_count FROM metrics WHERE server_id = ? AND timestamp > ? ORDER BY timestamp`
	if groupBy != "" {
		query = fmt.Sprintf(`SELECT MAX(timestamp), AVG(cpu_percent), AVG(memory_mb), MAX(player_count)
			FROM metrics WHERE server_id = ? AND timestamp > ?
			GROUP BY %s ORDER BY MAX(timestamp)`, groupBy)
	}

	rows, err := db.Query(query, serverID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var p MetricPoint
		// An aggregate such as MAX(timestamp) has no declared column type, so
		// the driver hands it back as a string rather than converting it to a
		// time.Time the way it does for the raw DATETIME column.
		var rawTimestamp interface{}
		if err := rows.Scan(&rawTimestamp, &p.CPUPercent, &p.MemoryMB, &p.PlayerCount); err != nil {
			return nil, err
		}
		p.Timestamp, err = parseTimestamp(rawTimestamp)
		if err != nil {
			return nil, err
		}
		points = append(points, p)
	}

	return points, rows.Err()
}

// SQLite bucket expressions. Grouping by the hour plus a division of the minute
// keeps each bucket the intended width.
const (
	fiveMinuteBucket = `strftime('%Y-%m-%d %H', timestamp), CAST(strftime('%M', timestamp) AS INTEGER) / 5`
	hourBucket       = `strftime('%Y-%m-%d %H', timestamp)`
	fourHourBucket   = `strftime('%Y-%m-%d', timestamp), CAST(strftime('%H', timestamp) AS INTEGER) / 4`
)

// timestampLayouts covers what the sqlite driver writes for a time.Time as well
// as the formats a hand-written row might use.
var timestampLayouts = []string{
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05-07:00",
	"2006-01-02T15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
}

func parseTimestamp(value interface{}) (time.Time, error) {
	switch v := value.(type) {
	case time.Time:
		return v, nil
	case []byte:
		return parseTimestampString(string(v))
	case string:
		return parseTimestampString(v)
	case nil:
		return time.Time{}, fmt.Errorf("metrics: missing timestamp")
	default:
		return time.Time{}, fmt.Errorf("metrics: unsupported timestamp type %T", value)
	}
}

func parseTimestampString(value string) (time.Time, error) {
	for _, layout := range timestampLayouts {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts, nil
		}
	}
	return time.Time{}, fmt.Errorf("metrics: cannot parse timestamp %q", value)
}

func PurgeOldMetrics(db *sql.DB, olderThan time.Time) (int64, error) {
	result, err := db.Exec(`DELETE FROM metrics WHERE timestamp < ?`, olderThan)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
