package ui

import "time"

// nextTimerDelay returns the duration the UI should sleep before its next
// transition check. The result is the gap from now to target, clamped to
// [0, cap]. Capping preserves correctness across macOS sleep, during which
// monotonic timers pause and a long sleep would mask wake-time drift.
//
// Contract:
//
//	Preconditions: cap > 0; otherwise panics.
//	Postconditions: 0 <= result <= cap.
func nextTimerDelay(now, target time.Time, cap time.Duration) time.Duration {
	if cap <= 0 {
		panic("ui.nextTimerDelay: cap must be > 0")
	}
	d := target.Sub(now)
	if d <= 0 {
		return 0
	}
	if d > cap {
		return cap
	}
	return d
}

// wakeDetected reports whether the actual timer fire time lagged its
// scheduled fire time by more than threshold. A positive result is the
// signal to force a calendar refresh — the host likely slept and may have
// woken in a different day.
//
// Contract:
//
//	Preconditions: threshold > 0.
//	Postconditions: true iff actual.Sub(scheduled) > threshold.
func wakeDetected(scheduled, actual time.Time, threshold time.Duration) bool {
	if threshold <= 0 {
		panic("ui.wakeDetected: threshold must be > 0")
	}
	return actual.Sub(scheduled) > threshold
}
