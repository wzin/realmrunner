package metrics

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func newMetricsDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if err := InitMetricsTable(db); err != nil {
		t.Fatalf("InitMetricsTable: %v", err)
	}
	return db
}

// seedMetrics writes one sample every 10 minutes going back the given duration.
func seedMetrics(t *testing.T, db *sql.DB, serverID string, back time.Duration) int {
	t.Helper()

	count := 0
	for ts := time.Now().Add(-back); ts.Before(time.Now()); ts = ts.Add(10 * time.Minute) {
		err := InsertMetric(db, &Metric{
			ServerID:    serverID,
			Timestamp:   ts,
			CPUPercent:  12.5,
			MemoryMB:    1024,
			PlayerCount: 2,
			PlayerNames: []string{"Alice", "Bob"},
		})
		if err != nil {
			t.Fatalf("InsertMetric: %v", err)
		}
		count++
	}
	return count
}

// Every range the UI offers must return data. The bucketed ranges (24h, 7d,
// 30d) aggregate with MAX(timestamp), which the driver hands back as a string
// rather than a DATETIME column - so they must be read accordingly.
func TestGetMetricsHistoryAllRanges(t *testing.T) {
	db := newMetricsDB(t)
	seedMetrics(t, db, "srv-1", 40*24*time.Hour)

	for _, rangeStr := range []string{"1h", "24h", "7d", "30d"} {
		t.Run(rangeStr, func(t *testing.T) {
			points, err := GetMetricsHistory(db, "srv-1", rangeStr)
			if err != nil {
				t.Fatalf("GetMetricsHistory(%s): %v", rangeStr, err)
			}
			if len(points) < 2 {
				t.Fatalf("GetMetricsHistory(%s) returned %d points, want a series", rangeStr, len(points))
			}

			for _, p := range points {
				if p.Timestamp.IsZero() {
					t.Fatalf("GetMetricsHistory(%s) returned a point with no timestamp: %+v", rangeStr, p)
				}
				if p.MemoryMB == 0 {
					t.Errorf("GetMetricsHistory(%s) lost the memory value: %+v", rangeStr, p)
				}
			}

			// Points must come back in chronological order.
			for i := 1; i < len(points); i++ {
				if points[i].Timestamp.Before(points[i-1].Timestamp) {
					t.Fatalf("GetMetricsHistory(%s) returned unordered points", rangeStr)
				}
			}
		})
	}
}

// A 30-day window must cover the whole period, not collapse into a handful of
// buckets or stop at 7 days.
func TestGetMetricsHistoryRangeWindows(t *testing.T) {
	db := newMetricsDB(t)
	seedMetrics(t, db, "srv-1", 40*24*time.Hour)

	oldest := func(rangeStr string) time.Time {
		points, err := GetMetricsHistory(db, "srv-1", rangeStr)
		if err != nil {
			t.Fatalf("GetMetricsHistory(%s): %v", rangeStr, err)
		}
		if len(points) == 0 {
			t.Fatalf("GetMetricsHistory(%s) returned nothing", rangeStr)
		}
		return points[0].Timestamp
	}

	day := oldest("24h")
	week := oldest("7d")
	month := oldest("30d")

	if !week.Before(day) {
		t.Errorf("7d starts at %s, which is not earlier than 24h at %s", week, day)
	}
	if !month.Before(week) {
		t.Errorf("30d starts at %s, which is not earlier than 7d at %s", month, week)
	}
	if time.Since(month) < 25*24*time.Hour {
		t.Errorf("30d only goes back to %s", month)
	}
}

func TestGetMetricsHistoryUnknownServer(t *testing.T) {
	db := newMetricsDB(t)
	seedMetrics(t, db, "srv-1", 2*time.Hour)

	points, err := GetMetricsHistory(db, "missing", "24h")
	if err != nil {
		t.Fatalf("GetMetricsHistory: %v", err)
	}
	if len(points) != 0 {
		t.Errorf("got %d points for an unknown server", len(points))
	}
}

func TestPurgeOldMetrics(t *testing.T) {
	db := newMetricsDB(t)
	seedMetrics(t, db, "srv-1", 40*24*time.Hour)

	removed, err := PurgeOldMetrics(db, time.Now().Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("PurgeOldMetrics: %v", err)
	}
	if removed == 0 {
		t.Error("PurgeOldMetrics removed nothing although older rows exist")
	}

	points, err := GetMetricsHistory(db, "srv-1", "30d")
	if err != nil {
		t.Fatal(err)
	}
	if len(points) == 0 {
		t.Error("purging removed everything")
	}
}

func TestGetLatestMetric(t *testing.T) {
	db := newMetricsDB(t)

	latest, err := GetLatestMetric(db, "srv-1")
	if err != nil {
		t.Fatalf("GetLatestMetric: %v", err)
	}
	if latest != nil {
		t.Error("expected no metric for a server with no samples")
	}

	seedMetrics(t, db, "srv-1", 2*time.Hour)

	latest, err = GetLatestMetric(db, "srv-1")
	if err != nil {
		t.Fatalf("GetLatestMetric: %v", err)
	}
	if latest == nil {
		t.Fatal("GetLatestMetric returned nothing")
	}
	if len(latest.PlayerNames) != 2 {
		t.Errorf("player names = %v", latest.PlayerNames)
	}
}
