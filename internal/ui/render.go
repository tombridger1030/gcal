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
	schedule schedule.ScheduleState
	width    int
	height   int
	stale    bool
	revoked  bool
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

// formatBlockLine renders one block as one or more lines of plain text. No
// ANSI escapes are emitted; styling is applied by renderView around this
// text. Width is enforced exactly.
//
// Contract:
//
//	Preconditions: width >= 8.
//	Postconditions: every line's visual width is <= width and lines start
//	with either currentMarker (StatusCurrent) or idleMarker (otherwise).
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

	timeText := fmt.Sprintf("%s – %s",
		b.Event.Start.Format("15:04"),
		b.Event.End.Format("15:04"),
	)
	timeLine := truncateToWidth(prefix+timeText+suffix, width)

	titleLine := truncateToWidth(prefix+b.Event.Title, width)

	lines := []string{timeLine, titleLine}
	if b.Event.Location != "" {
		lines = append(lines, truncateToWidth(prefix+b.Event.Location, width))
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
	for i, blk := range state.schedule.Blocks {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(formatBlockLine(blk, state.width))
		b.WriteString("\n")
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
