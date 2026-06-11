package focus

import (
	"testing"
	"time"

	"github.com/tombridger1030/gcal/internal/schedule"
)

func focusAt(h, m int) time.Time {
	return time.Date(2026, 5, 13, h, m, 0, 0, time.Local)
}

func workBlock(title string, startHour, startMinute, endHour, endMinute int) schedule.Block {
	return schedule.Block{
		Event: schedule.Event{
			Title: title,
			Start: focusAt(startHour, startMinute),
			End:   focusAt(endHour, endMinute),
		},
	}
}

func TestMatchesWorkBlock(t *testing.T) {
	tests := []struct {
		title string
		want  bool
	}{
		{"Work", true},
		{"Deep work", true},
		{"leftover WORK", true},
		{"Focus block", false},
		{"Admin", false},
	}
	for _, tc := range tests {
		t.Run(tc.title, func(t *testing.T) {
			if got := MatchesWorkBlock(workBlock(tc.title, 9, 0, 10, 0)); got != tc.want {
				t.Errorf("MatchesWorkBlock(%q) = %v, want %v", tc.title, got, tc.want)
			}
		})
	}
}

func TestPromptCoversWorkBlock(t *testing.T) {
	blocks := []schedule.Block{
		workBlock("Work", 9, 0, 18, 0),
		workBlock("Lunch", 12, 0, 13, 0),
	}
	tests := []struct {
		name     string
		promptAt time.Time
		want     bool
	}{
		{"before work block", focusAt(9, 0), false},
		{"first completed work hour", focusAt(10, 0), true},
		{"inside work block", focusAt(15, 0), true},
		{"last completed work hour", focusAt(18, 0), true},
		{"after work block", focusAt(19, 0), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := PromptCoversWorkBlock(tc.promptAt, blocks); got != tc.want {
				t.Errorf("PromptCoversWorkBlock(%v) = %v, want %v", tc.promptAt, got, tc.want)
			}
		})
	}
}

func TestNextPromptAtReturnsNextWorkTopOfHour(t *testing.T) {
	blocks := []schedule.Block{
		workBlock("Work", 9, 0, 18, 0),
		workBlock("Lunch", 12, 0, 13, 0),
	}
	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{"before work skips pre-work hour", focusAt(8, 15), focusAt(10, 0)},
		{"strictly after exact prompt", focusAt(10, 0), focusAt(11, 0)},
		{"inside work day", focusAt(14, 30), focusAt(15, 0)},
		{"includes final work hour", focusAt(17, 30), focusAt(18, 0)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := NextPromptAt(tc.now, blocks)
			if !ok {
				t.Fatal("NextPromptAt ok=false, want true")
			}
			if !got.Equal(tc.want) {
				t.Fatalf("NextPromptAt(%v) = %v, want %v", tc.now, got, tc.want)
			}
			if !got.After(tc.now) {
				t.Errorf("result must be strictly after now: got %v now %v", got, tc.now)
			}
			if !PromptCoversWorkBlock(got, blocks) {
				t.Errorf("result must cover work: got %v", got)
			}
		})
	}
}

func TestNextPromptAtHandlesPartialWorkBlocks(t *testing.T) {
	blocks := []schedule.Block{workBlock("Client work", 9, 30, 11, 15)}
	tests := []struct {
		now  time.Time
		want time.Time
	}{
		{focusAt(9, 0), focusAt(10, 0)},
		{focusAt(10, 0), focusAt(11, 0)},
		{focusAt(11, 0), focusAt(12, 0)},
	}
	for _, tc := range tests {
		got, ok := NextPromptAt(tc.now, blocks)
		if !ok {
			t.Fatalf("NextPromptAt(%v) ok=false, want true", tc.now)
		}
		if !got.Equal(tc.want) {
			t.Errorf("NextPromptAt(%v) = %v, want %v", tc.now, got, tc.want)
		}
	}
}

func TestNextPromptAtReturnsFalseWhenNoFutureWorkPromptExists(t *testing.T) {
	tests := []struct {
		name   string
		now    time.Time
		blocks []schedule.Block
	}{
		{"no blocks", focusAt(9, 0), nil},
		{"no work title", focusAt(9, 0), []schedule.Block{workBlock("Focus", 9, 0, 10, 0)}},
		{"past work", focusAt(18, 1), []schedule.Block{workBlock("Work", 9, 0, 18, 0)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := NextPromptAt(tc.now, tc.blocks); ok {
				t.Errorf("NextPromptAt() = %v, true; want false", got)
			}
		})
	}
}

func TestNextPromptAtPanicsForZeroNow(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for zero now")
		}
	}()
	NextPromptAt(time.Time{}, []schedule.Block{workBlock("Work", 9, 0, 10, 0)})
}

func TestHourStartForPrompt(t *testing.T) {
	promptAt := focusAt(14, 37)
	want := focusAt(13, 0)
	if got := HourStartForPrompt(promptAt); !got.Equal(want) {
		t.Errorf("HourStartForPrompt(%v) = %v, want %v", promptAt, got, want)
	}
}
