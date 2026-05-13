package schedule

import (
	"testing"
	"time"
)

// at builds a local time on 2026-05-13 at the given HH:MM for tests.
func at(h, m int) time.Time {
	return time.Date(2026, 5, 13, h, m, 0, 0, time.Local)
}

func ev(title string, startH, startM, endH, endM int) Event {
	return Event{
		ID:    title,
		Title: title,
		Start: at(startH, startM),
		End:   at(endH, endM),
	}
}

func TestBuildStateClassifiesPastCurrentFuture(t *testing.T) {
	events := []Event{
		ev("standup", 9, 0, 9, 30),
		ev("design review", 10, 30, 11, 30),
		ev("lunch", 12, 0, 13, 0),
	}
	state := BuildState(events, at(10, 42))

	if got, want := len(state.Blocks), 3; got != want {
		t.Fatalf("blocks: got %d, want %d", got, want)
	}
	wantStatus := []Status{StatusPast, StatusCurrent, StatusFuture}
	for i, want := range wantStatus {
		if state.Blocks[i].Status != want {
			t.Errorf("block %d (%s): status %v, want %v",
				i, state.Blocks[i].Event.Title, state.Blocks[i].Status, want)
		}
	}
}

func TestBuildStateBoundariesAreInclusiveOfStart(t *testing.T) {
	// At 10:30:00 exactly, the 10:30-11:30 event becomes Current.
	state := BuildState([]Event{ev("design review", 10, 30, 11, 30)}, at(10, 30))
	if state.Blocks[0].Status != StatusCurrent {
		t.Errorf("at start instant should be Current, got %v", state.Blocks[0].Status)
	}
}

func TestBuildStateBoundariesAreExclusiveOfEnd(t *testing.T) {
	// At 11:30:00 exactly, the 10:30-11:30 event becomes Past.
	state := BuildState([]Event{ev("design review", 10, 30, 11, 30)}, at(11, 30))
	if state.Blocks[0].Status != StatusPast {
		t.Errorf("at end instant should be Past, got %v", state.Blocks[0].Status)
	}
}

func TestBuildStateSortsByStart(t *testing.T) {
	state := BuildState([]Event{
		ev("c", 14, 0, 15, 0),
		ev("a", 9, 0, 10, 0),
		ev("b", 12, 0, 13, 0),
	}, at(8, 0))

	got := []string{state.Blocks[0].Event.Title, state.Blocks[1].Event.Title, state.Blocks[2].Event.Title}
	want := []string{"a", "b", "c"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sort: got %v, want %v", got, want)
			return
		}
	}
}

func TestBuildStateClampsEventsAcrossMidnight(t *testing.T) {
	// Event from previous-day 22:00 to today's 02:00 becomes a 00:00-02:00 block.
	prevNight := time.Date(2026, 5, 12, 22, 0, 0, 0, time.Local)
	thisMorning := time.Date(2026, 5, 13, 2, 0, 0, 0, time.Local)
	state := BuildState([]Event{{
		ID: "long", Title: "long", Start: prevNight, End: thisMorning,
	}}, at(1, 0))

	if len(state.Blocks) != 1 {
		t.Fatalf("blocks: got %d, want 1", len(state.Blocks))
	}
	b := state.Blocks[0]
	if b.Event.Start != at(0, 0) {
		t.Errorf("start: got %v, want 00:00", b.Event.Start)
	}
	if b.Event.End != at(2, 0) {
		t.Errorf("end: got %v, want 02:00", b.Event.End)
	}
	if b.Status != StatusCurrent {
		t.Errorf("status: got %v, want Current", b.Status)
	}
}

func TestBuildStateDropsEventsOutsideDay(t *testing.T) {
	yesterday := Event{
		ID: "old", Title: "old",
		Start: time.Date(2026, 5, 12, 9, 0, 0, 0, time.Local),
		End:   time.Date(2026, 5, 12, 10, 0, 0, 0, time.Local),
	}
	tomorrow := Event{
		ID: "tmrw", Title: "tmrw",
		Start: time.Date(2026, 5, 14, 9, 0, 0, 0, time.Local),
		End:   time.Date(2026, 5, 14, 10, 0, 0, 0, time.Local),
	}
	state := BuildState([]Event{yesterday, tomorrow}, at(12, 0))
	if len(state.Blocks) != 0 {
		t.Errorf("expected no blocks, got %d", len(state.Blocks))
	}
}

func TestBuildStateSeparatesAllDay(t *testing.T) {
	allDay := Event{
		ID: "ooo", Title: "OOO",
		Start:  time.Date(2026, 5, 13, 0, 0, 0, 0, time.Local),
		End:    time.Date(2026, 5, 14, 0, 0, 0, 0, time.Local),
		AllDay: true,
	}
	timed := ev("standup", 9, 0, 9, 30)
	state := BuildState([]Event{allDay, timed}, at(10, 0))

	if len(state.AllDay) != 1 || state.AllDay[0].Title != "OOO" {
		t.Errorf("AllDay: got %+v, want one OOO", state.AllDay)
	}
	if len(state.Blocks) != 1 || state.Blocks[0].Event.Title != "standup" {
		t.Errorf("Blocks: got %+v, want one standup", state.Blocks)
	}
}

func TestBuildStateEmptyDay(t *testing.T) {
	state := BuildState(nil, at(10, 0))
	if len(state.Blocks) != 0 || len(state.AllDay) != 0 {
		t.Errorf("expected empty state, got %+v", state)
	}
	if state.Day != at(0, 0) {
		t.Errorf("Day: got %v, want 2026-05-13 00:00", state.Day)
	}
}

func TestNextTransitionReturnsCurrentEventEnd(t *testing.T) {
	state := BuildState([]Event{
		ev("standup", 9, 0, 9, 30),
		ev("design review", 10, 30, 11, 30),
		ev("lunch", 12, 0, 13, 0),
	}, at(10, 42))

	next := state.NextTransition(at(10, 42))
	if next != at(11, 30) {
		t.Errorf("next: got %v, want 11:30 (current event end)", next)
	}
}

func TestNextTransitionReturnsNextEventStart(t *testing.T) {
	// Between events: 09:30 standup ended, 10:30 design review hasn't started.
	state := BuildState([]Event{
		ev("standup", 9, 0, 9, 30),
		ev("design review", 10, 30, 11, 30),
	}, at(9, 45))

	next := state.NextTransition(at(9, 45))
	if next != at(10, 30) {
		t.Errorf("next: got %v, want 10:30 (next event start)", next)
	}
}

func TestNextTransitionFallsBackToMidnight(t *testing.T) {
	// All events done; only midnight remains.
	state := BuildState([]Event{ev("standup", 9, 0, 9, 30)}, at(20, 0))
	next := state.NextTransition(at(20, 0))
	want := time.Date(2026, 5, 14, 0, 0, 0, 0, time.Local)
	if next != want {
		t.Errorf("next: got %v, want %v (midnight)", next, want)
	}
}

func TestNextTransitionEmptyDayIsMidnight(t *testing.T) {
	state := BuildState(nil, at(8, 0))
	next := state.NextTransition(at(8, 0))
	want := time.Date(2026, 5, 14, 0, 0, 0, 0, time.Local)
	if next != want {
		t.Errorf("next: got %v, want %v (midnight)", next, want)
	}
}

func TestNextTransitionDSTSafeForNextMidnight(t *testing.T) {
	// startOfNextDay must use time.Date(d+1) rather than +24h so DST
	// transitions don't drift the boundary.
	state := BuildState(nil, at(23, 0))
	next := state.NextTransition(at(23, 0))
	if next.Hour() != 0 || next.Minute() != 0 || next.Second() != 0 {
		t.Errorf("next midnight is not aligned: %v", next)
	}
	if next.Day() != 14 {
		t.Errorf("next midnight should land on the 14th, got day=%d", next.Day())
	}
}

// --- DBC invariant tests ---

func TestBuildStatePostconditionDayIsLocalMidnight(t *testing.T) {
	state := BuildState(nil, at(15, 23))
	want := at(0, 0)
	if !state.Day.Equal(want) {
		t.Errorf("Day: got %v, want %v", state.Day, want)
	}
}

func TestBuildStatePostconditionBlocksAreSortedAndClamped(t *testing.T) {
	dayBefore := time.Date(2026, 5, 12, 22, 0, 0, 0, time.Local)
	dayAfter := time.Date(2026, 5, 14, 2, 0, 0, 0, time.Local)
	state := BuildState([]Event{
		{ID: "a", Start: at(14, 0), End: at(15, 0)},
		{ID: "b", Start: at(9, 0), End: at(10, 0)},
		{ID: "spanning", Start: dayBefore, End: dayAfter},
	}, at(12, 0))

	dayStart := state.Day
	dayEnd := time.Date(state.Day.Year(), state.Day.Month(), state.Day.Day()+1, 0, 0, 0, 0, time.Local)

	for i := 1; i < len(state.Blocks); i++ {
		if state.Blocks[i].Event.Start.Before(state.Blocks[i-1].Event.Start) {
			t.Errorf("blocks not sorted at index %d", i)
		}
	}
	for _, b := range state.Blocks {
		if b.Event.Start.Before(dayStart) || b.Event.End.After(dayEnd) {
			t.Errorf("block exceeds day window: %+v (day=%v..%v)", b.Event, dayStart, dayEnd)
		}
		if !b.Event.End.After(b.Event.Start) {
			t.Errorf("zero-or-negative-duration block: %+v", b.Event)
		}
		if b.Event.AllDay {
			t.Errorf("AllDay event leaked into Blocks: %+v", b.Event)
		}
	}
}

func TestBuildStatePanicsOnZeroNow(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on zero now")
		}
	}()
	BuildState(nil, time.Time{})
}

func TestNextTransitionPanicsOnZeroNow(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on zero now")
		}
	}()
	BuildState(nil, at(12, 0)).NextTransition(time.Time{})
}

func TestNextTransitionPostconditionStrictlyAfterNow(t *testing.T) {
	state := BuildState([]Event{
		ev("standup", 9, 0, 9, 30),
		ev("design review", 10, 30, 11, 30),
	}, at(8, 0))

	for _, now := range []time.Time{at(8, 0), at(9, 0), at(9, 30), at(10, 30), at(11, 30), at(23, 59)} {
		next := state.NextTransition(now)
		if !next.After(now) {
			t.Errorf("postcondition violated: NextTransition(%v) = %v (not strictly after)", now, next)
		}
	}
}

func TestBuildStateDoesNotMutateInput(t *testing.T) {
	in := []Event{ev("a", 9, 0, 10, 0), ev("b", 10, 0, 11, 0)}
	snapshot := []Event{in[0], in[1]}
	BuildState(in, at(9, 30))
	for i := range in {
		if in[i] != snapshot[i] {
			t.Errorf("input mutated at index %d: got %+v, want %+v", i, in[i], snapshot[i])
		}
	}
}
