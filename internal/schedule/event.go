// Package schedule contains the pure domain model for a day of calendar events.
//
// It exposes two entry points: BuildState, which projects a slice of events
// onto a "now" instant to produce a renderable ScheduleState; and
// ScheduleState.NextTransition, which returns the next moment the projection
// would change. Together they let the UI repaint exactly when something
// visually changes — never on a timer tick.
//
// This package performs no IO and depends only on the standard library, so
// every behavior here is exercised by table-driven tests.
package schedule

import "time"

// Event is the domain representation of a single calendar event, normalized
// so that callers never need to think about RFC3339 strings, Google's
// EventDateTime split, or non-local timezones.
type Event struct {
	ID       string
	Title    string
	Start    time.Time // always in time.Local
	End      time.Time // always in time.Local
	AllDay   bool
	Location string
}

// Status is where an event sits relative to "now".
type Status int

const (
	StatusPast Status = iota
	StatusCurrent
	StatusFuture
)

// Block is a timed event paired with its current status. All-day events are
// not rendered as blocks — they live in ScheduleState.AllDay.
type Block struct {
	Event  Event
	Status Status
}

// ScheduleState is everything the UI needs to render one day. It is
// recomputed at every transition; the cost is dominated by sorting the
// event list, which is bounded (a typical day has <30 events).
type ScheduleState struct {
	Day    time.Time // local midnight of the day represented
	Blocks []Block   // timed events, sorted by Start, clamped to [Day, Day+24h]
	AllDay []Event   // all-day events for the banner
}
