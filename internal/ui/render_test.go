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

// formatBlockLine emits a single compact line per block ("HH:MM–HH:MM
// <title>") when there is no location, so a busy day still fits in a
// pinned narrow terminal. Location adds one indented continuation line.
func TestFormatBlockLineIsSingleLineWithoutLocation(t *testing.T) {
	b := schedule.Block{
		Event:  schedule.Event{Title: "Standup", Start: at(9, 0), End: at(9, 30)},
		Status: schedule.StatusFuture,
	}
	got := formatBlockLine(b, 32)
	if n := strings.Count(got, "\n"); n != 0 {
		t.Errorf("expected single line, got %d newlines: %q", n, got)
	}
	if !strings.Contains(got, "09:00") || !strings.Contains(got, "09:30") {
		t.Errorf("expected both start and end times on one line: %q", got)
	}
}

func TestFormatBlockLineAddsLocationLine(t *testing.T) {
	b := schedule.Block{
		Event: schedule.Event{
			Title: "Lunch w/ Sam", Location: "Tartine",
			Start: at(12, 0), End: at(13, 0),
		},
		Status: schedule.StatusFuture,
	}
	got := formatBlockLine(b, 32)
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines with location, got %d: %q", len(lines), got)
	}
	if !strings.Contains(lines[1], "Tartine") {
		t.Errorf("expected location on second line: %q", lines[1])
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
	// Titles are kept short enough to survive the current-event
	// title budget at width 32 (which the compact layout reserves
	// for the NOW suffix). Truncation behavior under narrow widths
	// is exercised separately by TestFormatBlockLineRespectsWidth.
	state := schedule.BuildState([]schedule.Event{
		{ID: "1", Title: "Standup", Start: at(9, 0), End: at(9, 30)},
		{ID: "2", Title: "Design", Start: at(10, 30), End: at(11, 30)},
	}, at(10, 42))

	out := renderView(viewState{schedule: state, width: 32, height: 30})
	if !strings.Contains(out, "Standup") || !strings.Contains(out, "Design") {
		t.Errorf("missing event titles in output:\n%s", out)
	}
	if !strings.Contains(out, "May 13") {
		t.Errorf("missing date header in output:\n%s", out)
	}
}

// A busy day of 14 events must fit inside the README's recommended
// 30-row pane. This is the regression that motivated the compact
// single-line layout — prior to it a 14-event day spilled to ~58 rows
// and FORGE (a non-altscreen-aware terminal) scrolled the earliest
// events off the top.
func TestRenderViewFitsBusyDayIn30Rows(t *testing.T) {
	events := []schedule.Event{
		{ID: "1", Title: "Wakeup", Start: at(7, 0), End: at(7, 30)},
		{ID: "2", Title: "Heavy Focus Block", Start: at(7, 30), End: at(11, 30)},
		{ID: "3", Title: "Matcha+Walk", Start: at(11, 30), End: at(12, 0)},
		{ID: "4", Title: "Meetings", Start: at(12, 0), End: at(14, 0)},
		{ID: "5", Title: "Shake + Walk", Start: at(14, 0), End: at(14, 30)},
		{ID: "6", Title: "Heavy Focus Block", Start: at(14, 30), End: at(17, 30)},
		{ID: "7", Title: "Heavy Work", Start: at(17, 30), End: at(18, 30)},
		{ID: "8", Title: "Pack + Leave for BJJ", Start: at(18, 30), End: at(19, 30)},
		{ID: "9", Title: "BJJ", Start: at(19, 30), End: at(21, 0)},
		{ID: "10", Title: "Shower + clean", Start: at(21, 0), End: at(21, 15)},
		{ID: "11", Title: "Meal 3", Start: at(21, 15), End: at(22, 0)},
		{ID: "12", Title: "Leftover Work", Start: at(22, 0), End: at(23, 30)},
	}
	state := schedule.BuildState(events, at(15, 0))
	out := renderView(viewState{schedule: state, width: 32, height: 30})
	if n := strings.Count(out, "\n"); n > 30 {
		t.Errorf("busy day overflows 30-row pane: %d lines\n%s", n, out)
	}
	// Sanity at a wider width every title is intact (no truncation),
	// so we know the lines-budget check above isn't passing by
	// silently dropping events from the output.
	wide := renderView(viewState{schedule: state, width: 64, height: 30})
	for _, e := range events {
		if !strings.Contains(wide, e.Title) {
			t.Errorf("missing title %q at width 64:\n%s", e.Title, wide)
		}
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

func TestRenderViewFocusError(t *testing.T) {
	state := schedule.BuildState(nil, at(10, 0))
	out := renderView(viewState{schedule: state, width: 32, height: 12, focusErr: true})
	if !strings.Contains(out, "focus log failed") {
		t.Errorf("focus error missing:\n%s", out)
	}
}

func TestRenderFocusPromptShowsHourAndRespectsWidth(t *testing.T) {
	const width = 24
	out := renderFocusPrompt(width, 12, at(14, 0))
	for _, line := range strings.Split(out, "\n") {
		if utf8Width(line) > width {
			t.Errorf("line exceeds width %d: %q", width, line)
		}
	}
	for _, want := range []string{"How focused?", "14:00-15:00", "1  distracted", "5  deep focus"} {
		if !strings.Contains(out, want) {
			t.Errorf("prompt missing %q:\n%s", want, out)
		}
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
