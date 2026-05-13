package schedule

import (
	"sort"
	"time"
)

// BuildState projects the given events onto the day containing now and
// classifies each timed event as Past, Current, or Future. Events that
// straddle midnight are clamped to the day's [00:00, 24:00) window so the UI
// renders them at the day's edge instead of falling off the timeline.
// All-day events are surfaced separately for banner rendering.
//
// Contract:
//
//	Preconditions:
//	  - now must be a non-zero time. Panics on zero time.
//	  - events may be nil or empty.
//	  - The input slice is not modified (treated as read-only).
//
//	Postconditions:
//	  - state.Day equals local midnight of the day containing now.
//	  - state.Blocks are sorted by Event.Start ascending.
//	  - For every block, state.Day <= Event.Start < Event.End <= state.Day+24h.
//	  - For every block, Event.AllDay is false.
//	  - For every all-day entry, Event.AllDay is true and the event
//	    overlaps [state.Day, state.Day+24h).
func BuildState(events []Event, now time.Time) ScheduleState {
	if now.IsZero() {
		panic("schedule.BuildState: now is zero")
	}
	now = now.Local()
	day := startOfDay(now)
	dayEnd := startOfNextDay(day) // DST-safe: not day.Add(24h)

	state := ScheduleState{Day: day}

	for _, e := range events {
		if e.AllDay {
			if dayOverlapsAllDay(e, day, dayEnd) {
				state.AllDay = append(state.AllDay, e)
			}
			continue
		}

		start := e.Start.Local()
		end := e.End.Local()
		if !end.After(day) || !start.Before(dayEnd) {
			continue // doesn't overlap this day
		}

		clamped := e
		clamped.Start = laterOf(start, day)
		clamped.End = earlierOf(end, dayEnd)
		if !clamped.End.After(clamped.Start) {
			continue
		}

		state.Blocks = append(state.Blocks, Block{
			Event:  clamped,
			Status: classify(clamped.Start, clamped.End, now),
		})
	}

	sort.SliceStable(state.Blocks, func(i, j int) bool {
		return state.Blocks[i].Event.Start.Before(state.Blocks[j].Event.Start)
	})
	return state
}

// NextTransition returns the soonest moment strictly after now at which
// BuildState's output would visually change: an upcoming block's start, a
// current block's end, or the next local midnight.
//
// Contract:
//
//	Preconditions:
//	  - now must be non-zero.
//	  - s must be a state previously produced by BuildState (its Day field
//	    is required to compute the midnight fallback). The receiver is
//	    treated as read-only.
//
//	Postconditions:
//	  - The returned time is strictly after now.
//	  - The returned time is no later than startOfNextDay(s.Day).
//	  - The returned time equals the earliest of: any block boundary
//	    (Start or End) that lies in (now, startOfNextDay(s.Day)], and
//	    startOfNextDay(s.Day) itself.
func (s ScheduleState) NextTransition(now time.Time) time.Time {
	if now.IsZero() {
		panic("schedule.NextTransition: now is zero")
	}
	now = now.Local()
	next := startOfNextDay(s.Day) // midnight is always a candidate

	for _, b := range s.Blocks {
		if b.Event.Start.After(now) && b.Event.Start.Before(next) {
			next = b.Event.Start
		}
		if b.Event.End.After(now) && b.Event.End.Before(next) {
			next = b.Event.End
		}
	}
	if !next.After(now) {
		// Postcondition violation: caller invoked NextTransition at or
		// after the day's end. Treat the next day's midnight as the
		// answer to keep the contract intact.
		next = time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	}
	return next
}

func classify(start, end, now time.Time) Status {
	switch {
	case !end.After(now):
		return StatusPast
	case start.After(now):
		return StatusFuture
	default:
		return StatusCurrent
	}
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func startOfNextDay(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day()+1, 0, 0, 0, 0, day.Location())
}

func laterOf(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func earlierOf(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// dayOverlapsAllDay returns true when an all-day event covers any portion of
// the given day. All-day Google events use date-only fields where Start is
// the inclusive start day at 00:00 and End is the exclusive day after the
// last covered day at 00:00.
func dayOverlapsAllDay(e Event, day, dayEnd time.Time) bool {
	return e.Start.Before(dayEnd) && e.End.After(day)
}
