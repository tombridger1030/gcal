package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tombridger1030/gcal/internal/schedule"
)

// viewState is the input to renderView. It is the smallest set of values
// the renderer needs; the model assembles it on each tea View() call.
type viewState struct {
	schedule       schedule.ScheduleState
	width          int
	height         int
	stale          bool
	revoked        bool
	focusPrompting bool
	focusHourStart time.Time
	focusErr       bool
}

const (
	currentMarker = "▌ "
	idleMarker    = "  "
	nowSuffix     = "  ◀ NOW"
)

// formatHeader renders the one-line date header.
//
// Postcondition: contains weekday abbrev and "Mon DD"; no trailing newline.
func formatHeader(day time.Time) string {
	return day.Format("Mon · Jan 2")
}

// formatBlockLine renders one block as a compact single-line entry
// "<marker><HH:MM–HH:MM>  <title>" plus an indented second line for
// location when present. The current block right-aligns the NOW suffix
// against the right edge so it lines up across the column.
//
// Contract:
//
//	Preconditions: width >= 8.
//	Postconditions:
//	  - every emitted line's visual width is <= width.
//	  - every line starts with currentMarker (StatusCurrent) or
//	    idleMarker (otherwise).
//	  - the " ◀ NOW" suffix only appears when status == StatusCurrent
//	    (and only when width is wide enough to fit it without erasing
//	    the title; under heavy truncation it may be dropped).
func formatBlockLine(b schedule.Block, width int) string {
	if width < 8 {
		panic("ui.formatBlockLine: width must be >= 8")
	}

	prefix := idleMarker
	suffix := ""
	if b.Status == schedule.StatusCurrent {
		prefix = currentMarker
		suffix = nowSuffix
	}

	timeText := fmt.Sprintf("%s–%s",
		b.Event.Start.Format("15:04"),
		b.Event.End.Format("15:04"),
	)
	const gap = "  "
	prefixLen := utf8Width(prefix)
	timeLen := utf8Width(timeText)
	gapLen := utf8Width(gap)
	suffixLen := utf8Width(suffix)

	titleBudget := width - prefixLen - timeLen - gapLen - suffixLen
	if titleBudget < 1 {
		// Width is too tight to fit the NOW suffix; drop it and retry.
		suffix = ""
		suffixLen = 0
		titleBudget = width - prefixLen - timeLen - gapLen
		if titleBudget < 1 {
			titleBudget = 1
		}
	}

	title := truncateToWidth(b.Event.Title, titleBudget)
	if b.Status == schedule.StatusCurrent && suffix != "" {
		// Pad the title column so the suffix right-aligns at the
		// rightmost cell, keeping NOW markers in a tidy column when
		// stacked across events.
		title += strings.Repeat(" ", titleBudget-utf8Width(title))
	}
	line := truncateToWidth(prefix+timeText+gap+title+suffix, width)

	lines := []string{line}
	if b.Event.Location != "" {
		// Indent location under the title column, keeping the marker
		// prefix so current-event continuity reads vertically.
		locIndent := strings.Repeat(" ", timeLen+gapLen)
		lines = append(lines, truncateToWidth(prefix+locIndent+b.Event.Location, width))
	}
	return strings.Join(lines, "\n")
}

// renderView produces the complete TUI text. It is pure: no time.Now, no
// IO. Tests cover content; the Bubble Tea model wraps the result.
//
// Contract:
//
//	Postconditions: every line's visual width is <= max(state.width, 8).
func renderView(state viewState) string {
	if state.width < 8 {
		msg := "narrow"
		if state.width < utf8.RuneCountInString(msg) {
			msg = strings.Repeat(".", max(state.width, 1))
		}
		return centered(msg, max(state.width, 1), max(state.height, 1))
	}
	if state.focusPrompting {
		return renderFocusPrompt(state.width, state.height, state.focusHourStart)
	}

	var b strings.Builder
	header := formatHeader(state.schedule.Day)
	if state.stale {
		// Right-align the stale indicator in the header.
		pad := state.width - utf8.RuneCountInString(header) - 1
		if pad < 1 {
			pad = 1
		}
		header = header + strings.Repeat(" ", pad) + "~"
	}
	b.WriteString(truncateToWidth(header, state.width))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", state.width))
	b.WriteString("\n")
	if state.focusErr {
		b.WriteString(truncateToWidth("! focus log failed", state.width))
		b.WriteString("\n")
	}

	if state.revoked {
		b.WriteString("\n")
		b.WriteString(centered("Calendar access revoked.", state.width, 1))
		b.WriteString("\n")
		b.WriteString(centered("Run:  gcal --login", state.width, 1))
		return b.String()
	}

	for _, e := range state.schedule.AllDay {
		line := truncateToWidth("all-day: "+e.Title, state.width)
		b.WriteString(line)
		b.WriteString("\n")
	}

	if len(state.schedule.Blocks) == 0 {
		b.WriteString("\n")
		b.WriteString(centered("No events today", state.width, 1))
		return b.String()
	}

	b.WriteString("\n")
	for _, blk := range state.schedule.Blocks {
		b.WriteString(formatBlockLine(blk, state.width))
		b.WriteString("\n")
	}
	return b.String()
}

// renderFocusPrompt renders the narrow in-TUI rating prompt.
func renderFocusPrompt(width, height int, hourStart time.Time) string {
	if width < 8 {
		return centered("narrow", max(width, 1), max(height, 1))
	}
	hourEnd := hourStart.Add(time.Hour)
	ruleWidth := min(width, 13)
	lines := []string{
		"How focused?",
		fmt.Sprintf("last hour %s-%s", hourStart.Format("15:04"), hourEnd.Format("15:04")),
		strings.Repeat("─", ruleWidth),
		"1  distracted",
		"3  mixed",
		"5  deep focus",
		"",
		"1-5 log · s skip",
	}

	var b strings.Builder
	for i, line := range lines {
		if line == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString(centered(truncateToWidth(line, width), width, 1))
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func truncateToWidth(s string, width int) string {
	if utf8.RuneCountInString(s) <= width {
		return s
	}
	if width <= 1 {
		return strings.Repeat(".", width)
	}
	runes := []rune(s)
	return string(runes[:width-1]) + "…"
}

func utf8Width(s string) int { return utf8.RuneCountInString(s) }

func centered(s string, width, _ int) string {
	if utf8.RuneCountInString(s) >= width {
		return truncateToWidth(s, width)
	}
	pad := (width - utf8.RuneCountInString(s)) / 2
	return strings.Repeat(" ", pad) + s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
