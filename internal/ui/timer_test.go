package ui

import (
	"testing"
	"time"
)

// nextTimerDelay must:
//   - return 0 when target is at or before now (caller fires immediately),
//   - return target-now when that gap is within cap,
//   - return cap when the gap exceeds cap (sleep-until-boundary capped for
//     macOS sleep-skew robustness).
//
// Contract preconditions:
//   - cap > 0; otherwise panic.
func TestNextTimerDelay(t *testing.T) {
	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.Local)
	const cap5 = 5 * time.Minute

	tests := []struct {
		name   string
		target time.Time
		cap    time.Duration
		want   time.Duration
	}{
		{"target equals now", now, cap5, 0},
		{"target before now", now.Add(-1 * time.Minute), cap5, 0},
		{"target within cap", now.Add(30 * time.Second), cap5, 30 * time.Second},
		{"target exactly at cap", now.Add(cap5), cap5, cap5},
		{"target past cap", now.Add(20 * time.Minute), cap5, cap5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := nextTimerDelay(now, tc.target, tc.cap)
			if got != tc.want {
				t.Errorf("nextTimerDelay(%v, %v, %v) = %v, want %v",
					now, tc.target, tc.cap, got, tc.want)
			}
		})
	}
}

func TestNextTimerDelayPanicsOnNonPositiveCap(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for cap <= 0")
		}
	}()
	now := time.Now()
	nextTimerDelay(now, now.Add(time.Second), 0)
}

// wakeDetected returns true when the timer fired noticeably later than its
// scheduled fire time, suggesting the host slept (Go monotonic timers pause
// during macOS sleep). Threshold guards against ordinary jitter.
func TestWakeDetected(t *testing.T) {
	scheduled := time.Date(2026, 5, 13, 10, 0, 0, 0, time.Local)
	threshold := time.Minute

	tests := []struct {
		name   string
		actual time.Time
		want   bool
	}{
		{"on time", scheduled, false},
		{"jitter under threshold", scheduled.Add(30 * time.Second), false},
		{"at threshold", scheduled.Add(threshold), false},
		{"well past threshold", scheduled.Add(2 * time.Minute), true},
		{"hours late (laptop slept)", scheduled.Add(6 * time.Hour), true},
		{"actual before scheduled", scheduled.Add(-time.Second), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := wakeDetected(scheduled, tc.actual, threshold); got != tc.want {
				t.Errorf("wakeDetected(%v, %v, %v) = %v, want %v",
					scheduled, tc.actual, threshold, got, tc.want)
			}
		})
	}
}
