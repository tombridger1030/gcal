// Package focus owns the focus check-in domain: work-block prompt math,
// journal entries, and the persistence contract for recorded ratings.
package focus

import (
	"strings"
	"time"

	"github.com/tombridger1030/gcal/internal/schedule"
)

const (
	// MinRating and MaxRating define the only valid focus scale accepted by
	// the journal and UI.
	MinRating = 1
	MaxRating = 5
)

// Entry is one answered focus prompt. HourStart is the beginning of the
// hour being rated in local time; Rating must be in [1, 5]; LoggedAt is
// when the user answered.
type Entry struct {
	HourStart time.Time `json:"hour_start"`
	Rating    int       `json:"rating"`
	LoggedAt  time.Time `json:"logged_at"`
}

// ValidRating reports whether r is accepted by the 1-5 focus scale.
func ValidRating(r int) bool {
	return r >= MinRating && r <= MaxRating
}

// WorkTitleFragment is the event-title marker that makes a calendar block
// eligible for focus prompts.
const WorkTitleFragment = "work"

// MatchesWorkBlock reports whether b's event title opts the block into focus
// prompts. Matching is case-insensitive and substring-based, so "Work",
// "Deep work", and "workout" all match.
func MatchesWorkBlock(b schedule.Block) bool {
	return strings.Contains(strings.ToLower(b.Event.Title), WorkTitleFragment)
}

// PromptCoversWorkBlock reports whether a prompt at promptAt rates a
// completed hour that overlaps any matching work block.
func PromptCoversWorkBlock(promptAt time.Time, blocks []schedule.Block) bool {
	hourStart := HourStartForPrompt(promptAt)
	for _, b := range blocks {
		if !MatchesWorkBlock(b) {
			continue
		}
		if b.Event.Start.Before(promptAt) && b.Event.End.After(hourStart) {
			return true
		}
	}
	return false
}

// NextPromptAt returns the earliest local top-of-hour strictly after now
// whose completed prior hour overlaps a work-titled block.
//
// Contract:
//
//	Preconditions:
//	  - now must be non-zero. Panics on zero time.
//	  - blocks must come from schedule.BuildState for the represented day.
//	Postconditions:
//	  - if ok, result.After(now) && PromptCoversWorkBlock(result, blocks).
//	  - if !ok, no future top-of-hour in the supplied blocks' day overlaps
//	    a work-titled block.
func NextPromptAt(now time.Time, blocks []schedule.Block) (result time.Time, ok bool) {
	if now.IsZero() {
		panic("focus.NextPromptAt: now is zero")
	}
	localNow := now.In(time.Local)
	limit, ok := promptSearchLimit(blocks)
	if !ok {
		return time.Time{}, false
	}
	for candidate := nextTopOfHour(localNow); !candidate.After(limit); candidate = nextTopOfHour(candidate) {
		if PromptCoversWorkBlock(candidate, blocks) {
			return candidate, true
		}
	}
	return time.Time{}, false
}

// HourStartForPrompt returns the start of the completed local hour being
// rated by a prompt fired at promptAt. Minutes and seconds on promptAt are
// ignored.
func HourStartForPrompt(promptAt time.Time) time.Time {
	local := promptAt.In(time.Local)
	return time.Date(local.Year(), local.Month(), local.Day(), local.Hour()-1, 0, 0, 0, time.Local)
}

func nextTopOfHour(t time.Time) time.Time {
	local := t.In(time.Local)
	return time.Date(local.Year(), local.Month(), local.Day(), local.Hour()+1, 0, 0, 0, time.Local)
}

func promptSearchLimit(blocks []schedule.Block) (time.Time, bool) {
	var latest time.Time
	for _, b := range blocks {
		if !MatchesWorkBlock(b) {
			continue
		}
		end := b.Event.End.In(time.Local)
		if latest.IsZero() || end.After(latest) {
			latest = end
		}
	}
	if latest.IsZero() {
		return time.Time{}, false
	}
	return nextTopOfHour(latest.Add(-time.Nanosecond)), true
}
