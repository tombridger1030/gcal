package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tombridger1030/gcal/internal/calendar"
	"github.com/tombridger1030/gcal/internal/schedule"
)

// nowFn is the model's clock indirection — tests inject a fixed instant so
// transition logic is deterministic.

func newTestModel(now time.Time, events []schedule.Event) model {
	m := newModel(func() time.Time { return now })
	m.width = 32
	m.height = 30
	m.events = events
	m.eventsAt = now
	m.recomputeState()
	return m
}

// fetchDoneMsg replaces the model's events and clears stale/revoked flags.
func TestModelUpdateFetchDoneReplacesEvents(t *testing.T) {
	now := time.Date(2026, 5, 13, 9, 0, 0, 0, time.Local)
	m := newTestModel(now, nil)
	m.stale = true
	m.revoked = true

	updated, _ := m.update(fetchDoneMsg{
		events: []schedule.Event{
			{ID: "1", Title: "Standup", Start: now, End: now.Add(30 * time.Minute)},
		},
		at: now,
	})
	mm := updated.(model)

	if len(mm.events) != 1 || mm.events[0].Title != "Standup" {
		t.Errorf("events not replaced: %+v", mm.events)
	}
	if mm.stale || mm.revoked {
		t.Errorf("stale/revoked not cleared: stale=%v revoked=%v", mm.stale, mm.revoked)
	}
	if mm.fetching {
		t.Errorf("fetching flag should be cleared, got true")
	}
	if len(mm.state.Blocks) != 1 {
		t.Errorf("schedule state not recomputed: blocks=%d", len(mm.state.Blocks))
	}
}

// fetchErrMsg with prior data: keep state, set stale=true.
func TestModelUpdateFetchErrSetsStaleWhenPriorDataExists(t *testing.T) {
	now := time.Date(2026, 5, 13, 9, 0, 0, 0, time.Local)
	m := newTestModel(now, []schedule.Event{
		{ID: "1", Title: "Standup", Start: now, End: now.Add(30 * time.Minute)},
	})

	updated, _ := m.update(fetchErrMsg{err: errors.New("network down"), stale: true})
	mm := updated.(model)

	if !mm.stale {
		t.Error("expected stale=true after fetch error")
	}
	if len(mm.events) != 1 {
		t.Errorf("events should be preserved on error: %+v", mm.events)
	}
}

// fetchErrMsg of type ErrTokenRevoked → revoked flag set, stop fetching.
func TestModelUpdateFetchErrTokenRevokedSetsRevoked(t *testing.T) {
	now := time.Date(2026, 5, 13, 9, 0, 0, 0, time.Local)
	m := newTestModel(now, nil)

	updated, _ := m.update(fetchErrMsg{err: calendar.ErrTokenRevoked})
	mm := updated.(model)

	if !mm.revoked {
		t.Error("expected revoked=true on ErrTokenRevoked")
	}
}

// transitionMsg recomputes state with new now, no fetch.
func TestModelUpdateTransitionRecomputesStatuses(t *testing.T) {
	day := time.Date(2026, 5, 13, 0, 0, 0, 0, time.Local)
	events := []schedule.Event{
		{ID: "early", Title: "early", Start: day.Add(9 * time.Hour), End: day.Add(10 * time.Hour)},
		{ID: "later", Title: "later", Start: day.Add(11 * time.Hour), End: day.Add(12 * time.Hour)},
	}
	atNine := day.Add(9 * time.Hour)
	m := newTestModel(atNine, events)
	if m.state.Blocks[0].Status != schedule.StatusCurrent {
		t.Fatalf("setup: expected first block Current at 09:00, got %v", m.state.Blocks[0].Status)
	}

	atEleven := day.Add(11 * time.Hour)
	m.now = func() time.Time { return atEleven }
	updated, _ := m.update(transitionMsg{at: atEleven})
	mm := updated.(model)

	if mm.state.Blocks[0].Status != schedule.StatusPast {
		t.Errorf("first block: got %v, want Past", mm.state.Blocks[0].Status)
	}
	if mm.state.Blocks[1].Status != schedule.StatusCurrent {
		t.Errorf("second block: got %v, want Current", mm.state.Blocks[1].Status)
	}
}

// transitionMsg whose 'at' lies past the current state's day → request a
// refresh (day rollover).
func TestModelUpdateTransitionTriggersRefreshOnDayRollover(t *testing.T) {
	prevDay := time.Date(2026, 5, 13, 23, 59, 0, 0, time.Local)
	m := newTestModel(prevDay, nil)

	nextDay := time.Date(2026, 5, 14, 0, 0, 0, 0, time.Local)
	m.now = func() time.Time { return nextDay }
	_, cmd := m.update(transitionMsg{at: nextDay})
	if cmd == nil {
		t.Fatal("expected a fetch command on day rollover, got nil")
	}
}

// Wake-detection: a transition that fires far past its scheduled time
// should also force a refresh, since the host likely slept.
func TestModelUpdateTransitionTriggersRefreshOnWake(t *testing.T) {
	now := time.Date(2026, 5, 13, 9, 0, 0, 0, time.Local)
	m := newTestModel(now, nil)
	m.scheduledAt = now.Add(time.Minute)

	// Fired 10 minutes after scheduled — definitely woke from sleep.
	wokenAt := now.Add(11 * time.Minute)
	m.now = func() time.Time { return wokenAt }
	_, cmd := m.update(transitionMsg{at: wokenAt})
	if cmd == nil {
		t.Error("expected a fetch command after wake-detection")
	}
}

// 'q' key produces tea.Quit.
func TestModelUpdateQuitOnQ(t *testing.T) {
	now := time.Date(2026, 5, 13, 9, 0, 0, 0, time.Local)
	m := newTestModel(now, nil)
	_, cmd := m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected a quit command on 'q', got nil")
	}
}

// 'r' key produces a fetch command (refresh now).
func TestModelUpdateRefreshOnR(t *testing.T) {
	now := time.Date(2026, 5, 13, 9, 0, 0, 0, time.Local)
	m := newTestModel(now, nil)
	_, cmd := m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil {
		t.Fatal("expected fetch command on 'r', got nil")
	}
}

// In-flight fetch must not be re-triggered.
func TestModelUpdateRefreshIgnoredWhileFetching(t *testing.T) {
	now := time.Date(2026, 5, 13, 9, 0, 0, 0, time.Local)
	m := newTestModel(now, nil)
	m.fetching = true
	_, cmd := m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd != nil {
		t.Error("expected no command when fetch is already in flight")
	}
}

// WindowSizeMsg updates dimensions.
func TestModelUpdateWindowSize(t *testing.T) {
	now := time.Date(2026, 5, 13, 9, 0, 0, 0, time.Local)
	m := newTestModel(now, nil)
	updated, _ := m.update(tea.WindowSizeMsg{Width: 50, Height: 40})
	mm := updated.(model)
	if mm.width != 50 || mm.height != 40 {
		t.Errorf("size: got (%d,%d), want (50,40)", mm.width, mm.height)
	}
}

// fetcherFn lets us drive the model from tests without a real Google client.
type fetcherFn func(ctx context.Context, day time.Time) ([]schedule.Event, error)

func (f fetcherFn) FetchDay(ctx context.Context, day time.Time) ([]schedule.Event, error) {
	return f(ctx, day)
}

// Compile-time assertion that model.update has the signature we expect from
// tea.Model. (The actual Update method delegates to update.)
var _ = func() bool {
	var m model
	_, _ = m.update(nil)
	return true
}
