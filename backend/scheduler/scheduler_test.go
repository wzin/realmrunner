package scheduler

import (
	"testing"
	"time"
)

func TestShouldRestart(t *testing.T) {
	tests := []struct {
		schedule string
		time     time.Time
		expected bool
	}{
		// Daily schedule
		{"04:00", time.Date(2026, 3, 29, 4, 0, 0, 0, time.UTC), true},
		{"04:00", time.Date(2026, 3, 29, 4, 1, 0, 0, time.UTC), false},
		{"04:00", time.Date(2026, 3, 29, 5, 0, 0, 0, time.UTC), false},
		{"23:30", time.Date(2026, 3, 29, 23, 30, 0, 0, time.UTC), true},
		{"23:30", time.Date(2026, 3, 29, 23, 31, 0, 0, time.UTC), false},

		// Interval schedule
		{"interval:6h", time.Date(2026, 3, 29, 0, 0, 0, 0, time.UTC), true},
		{"interval:6h", time.Date(2026, 3, 29, 6, 0, 0, 0, time.UTC), true},
		{"interval:6h", time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC), true},
		{"interval:6h", time.Date(2026, 3, 29, 3, 0, 0, 0, time.UTC), false},
		{"interval:6h", time.Date(2026, 3, 29, 6, 1, 0, 0, time.UTC), false},
		{"interval:12h", time.Date(2026, 3, 29, 0, 0, 0, 0, time.UTC), true},
		{"interval:12h", time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC), true},
		{"interval:12h", time.Date(2026, 3, 29, 6, 0, 0, 0, time.UTC), false},

		// Invalid
		{"", time.Date(2026, 3, 29, 0, 0, 0, 0, time.UTC), false},
		{"garbage", time.Date(2026, 3, 29, 0, 0, 0, 0, time.UTC), false},
	}

	for _, tt := range tests {
		result := shouldRestart(tt.schedule, tt.time)
		if result != tt.expected {
			t.Errorf("shouldRestart(%q, %s) = %v, want %v", tt.schedule, tt.time, result, tt.expected)
		}
	}
}
