package oinc

import (
	"testing"
	"time"
)

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"seconds", 45 * time.Second, "45s"},
		{"minutes", 12 * time.Minute, "12m"},
		{"hours exact", 3 * time.Hour, "3h"},
		{"hours and minutes", 3*time.Hour + 25*time.Minute, "3h 25m"},
		{"days exact", 48 * time.Hour, "2d"},
		{"days and hours", 50 * time.Hour, "2d 2h"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatUptime(tt.d)
			if got != tt.want {
				t.Errorf("formatUptime(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}
