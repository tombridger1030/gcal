package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/tombridger1030/gcal/internal/schedule"
)

func at(h, m int) time.Time {
	return time.Date(2026, 5, 13, h, m, 0, 0, time.Local)
}

// formatHeader produces a one-line "Weekday · Mon DD" string.
//
// Contract:
//
//	Postcondition: returned string contains the abbreviated weekday and
//	month-day; never has a trailing newline; pure (no time.Now usage).
func TestFormatHeader(t *testing.T) {
	got := formatHeader(at(0, 0))
	if !strings.Contains(got, "May 13") {
		t.Errorf("header missing month/day: %q", got)
	}
	if !strings.Contains(got, "Wed") {
		t.Errorf("header missing weekday Wed: %q", got)
	}
	if strings.HasSuffix(got, "\n") {
		t.Errorf("header has trailing newline: %q", got)
	}
}

// formatBlockLine renders one block as plain text (sans color), respecting
// width. Used by view rendering and verifiable independently of Lipgloss.
//
// Contract:
//
//	Preconditions: width >= 8 (callers must guard via narrow-mode rendering).
//	Postconditions:
//	  - returned text width does not exceed width (no ANSI inside).
//	  - contains "HH:MM" of the block's start time.
//	  - contains the (possibly truncated) title.
//	  - the current marker (" NOW") only appears when status == StatusCurrent.
func TestFormatBlockLineIncludesTimeAndTitle(t *testing.T) {
	b := schedule.Block{
		Event:  schedule.Event{Title: "Standup", Start: at(9, 0), End: at(9, 30)},
		Status: schedule.StatusFuture,
	}
	got := formatBlockLine(b, 32)
	if !strings.Contains(got, "09:00") {
		t.Errorf("missing start time: %q", got)
	}
	if !strings.Contains(got, "Standup") {
		t.Errorf("missing title: %q", got)
	}
	if strings.Contains(got, "NOW") {
		t.Errorf("future block should not be marked NOW: %q", got)
	}
}

func TestFormatBlockLineMarksCurrent(t *testing.T) {
	b := schedule.Block{
		Event:  schedule.Event{Title: "Design review", Start: at(10, 30), End: at(11, 30)},
		Status: schedule.StatusCurrent,
	}
	got := formatBlockLine(b, 32)
	if !strings.Contains(got, "NOW") {
		t.Errorf("current block should be marked NOW: %q", got)
	}
}

func TestFormatBlockLineRespectsWidth(t *testing.T) {
	b := schedule.Block{
		Event: schedule.Event{
			Title: "An extremely long event title that definitely will not fit",
			Start: at(14, 0), End: at(15, 0),
		},
		Status: schedule.StatusFuture,
	}
	const width = 24
	got := formatBlockLine(b, width)
	for _, line := range strings.Split(got, "\n") {
		if utf8Width(line) > width {
			t.Errorf("line exceeds width %d: %q (width %d)", width, line, utf8Width(line))
		}
	}
}

func TestFormatBlockLinePanicsOnNarrowWidth(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for width < 8")
		}
	}()
	b := schedule.Block{Event: schedule.Event{Title: "x", Start: at(9, 0), End: at(9, 30)}}
	formatBlockLine(b, 4)
}

// renderView is the top-level pure renderer. It produces the full TUI text
// from a viewState; tests verify content without coupling to layout exactly.
func TestRenderViewShowsAllBlocks(t *testing.T) {
	state := schedule.BuildState([]schedule.Event{
		{ID: "1", Title: "Standup", Start: at(9, 0), End: at(9, 30)},
		{ID: "2", Title: "Design review", Start: at(10, 30), End: at(11, 30)},
	}, at(10, 42))

	out := renderView(viewState{schedule: state, width: 32, height: 30})
	if !strings.Contains(out, "Standup") || !strings.Contains(out, "Design review") {
		t.Errorf("missing event titles in output:\n%s", out)
	}
	if !strings.Contains(out, "May 13") {
		t.Errorf("missing date header in output:\n%s", out)
	}
}

func TestRenderViewEmptyDay(t *testing.T) {
	state := schedule.BuildState(nil, at(10, 0))
	out := renderView(viewState{schedule: state, width: 32, height: 12})
	if !strings.Contains(out, "No events today") {
		t.Errorf("empty day should show placeholder:\n%s", out)
	}
}

func TestRenderViewAllDayBanner(t *testing.T) {
	allDay := schedule.Event{
		ID: "ooo", Title: "OOO",
		Start:  time.Date(2026, 5, 13, 0, 0, 0, 0, time.Local),
		End:    time.Date(2026, 5, 14, 0, 0, 0, 0, time.Local),
		AllDay: true,
	}
	state := schedule.BuildState([]schedule.Event{allDay}, at(10, 0))
	out := renderView(viewState{schedule: state, width: 32, height: 12})
	if !strings.Contains(out, "all-day") || !strings.Contains(out, "OOO") {
		t.Errorf("all-day banner missing:\n%s", out)
	}
}

func TestRenderViewStaleIndicator(t *testing.T) {
	state := schedule.BuildState(nil, at(10, 0))
	out := renderView(viewState{schedule: state, width: 32, height: 12, stale: true})
	if !strings.Contains(out, "~") {
		t.Errorf("stale indicator missing:\n%s", out)
	}
}

func TestRenderViewTokenRevoked(t *testing.T) {
	out := renderView(viewState{
		schedule: schedule.BuildState(nil, at(10, 0)),
		width:    32, height: 12, revoked: true,
	})
	if !strings.Contains(out, "--login") {
		t.Errorf("revoked view should mention --login:\n%s", out)
	}
}

func TestRenderViewTooNarrow(t *testing.T) {
	state := schedule.BuildState(nil, at(10, 0))
	out := renderView(viewState{schedule: state, width: 6, height: 12})
	if !strings.Contains(out, "narrow") {
		t.Errorf("very narrow width should report 'narrow':\n%s", out)
	}
}
