package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tombridger1030/gcal/internal/calendar"
	"github.com/tombridger1030/gcal/internal/focus"
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

func TestModelFocusTickStartsPromptAtTarget(t *testing.T) {
	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.Local)
	m, _, _ := newFocusTestModel(now)
	m.nextFocusAt = now
	m.focusInterval = time.Nanosecond

	updated, cmd := m.update(focusTickMsg{at: now, gen: m.focusGen})
	mm := updated.(model)

	if !mm.prompting {
		t.Fatal("expected focus prompt to be active")
	}
	if !mm.promptTarget.Equal(now) {
		t.Errorf("promptTarget=%v, want %v", mm.promptTarget, now)
	}
	if cmd == nil {
		t.Fatal("expected notify/schedule command")
	}
	if out := mm.View(); !strings.Contains(out, "How focused?") || !strings.Contains(out, "09:00-10:00") {
		t.Errorf("prompt view missing expected text:\n%s", out)
	}
}

func TestModelFocusTickSkipsMissedPromptAfterWake(t *testing.T) {
	target := time.Date(2026, 5, 13, 10, 0, 0, 0, time.Local)
	wokeAt := target.Add(2 * wakeThreshold)
	m, _, _ := newFocusTestModel(wokeAt)
	m.nextFocusAt = target
	m.focusInterval = time.Nanosecond

	updated, _ := m.update(focusTickMsg{at: wokeAt, gen: m.focusGen})
	mm := updated.(model)

	if mm.prompting {
		t.Fatal("missed focus hour should be skipped, not prompted")
	}
}

func TestModelFocusRatingRecordsEntryAndClearsPrompt(t *testing.T) {
	now := time.Date(2026, 5, 13, 10, 2, 0, 0, time.Local)
	target := time.Date(2026, 5, 13, 10, 0, 0, 0, time.Local)
	m, recorder, _ := newFocusTestModel(now)
	m.prompting = true
	m.promptTarget = target
	m.focusInterval = time.Nanosecond

	updated, cmd := m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	mm := updated.(model)
	if mm.prompting {
		t.Fatal("prompt should clear after rating")
	}
	runBatch(cmd)
	if len(recorder.entries) != 1 {
		t.Fatalf("recorded entries=%d, want 1", len(recorder.entries))
	}
	got := recorder.entries[0]
	if got.Rating != 3 {
		t.Errorf("rating=%d, want 3", got.Rating)
	}
	wantHour := time.Date(2026, 5, 13, 9, 0, 0, 0, time.Local)
	if !got.HourStart.Equal(wantHour) {
		t.Errorf("hourStart=%v, want %v", got.HourStart, wantHour)
	}
	if !got.LoggedAt.Equal(now) {
		t.Errorf("loggedAt=%v, want %v", got.LoggedAt, now)
	}
}

func TestModelFocusTickDropsStaleGenerationWithoutRearming(t *testing.T) {
	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.Local)
	m, _, _ := newFocusTestModel(now)
	m.nextFocusAt = now
	m.focusGen = 2

	updated, cmd := m.update(focusTickMsg{at: now, gen: 1})
	mm := updated.(model)

	if mm.prompting {
		t.Fatal("stale focus timer should not prompt")
	}
	if cmd != nil {
		t.Fatal("stale focus timer should not re-arm")
	}
}

func TestScheduleFocusPromptStampsGeneration(t *testing.T) {
	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.Local)
	m, _, _ := newFocusTestModel(now)
	m.nextFocusAt = now
	m.focusGen = 7

	msg := m.scheduleFocusPrompt()()
	tick, ok := msg.(focusTickMsg)
	if !ok {
		t.Fatalf("message type=%T, want focusTickMsg", msg)
	}
	if tick.gen != 7 {
		t.Errorf("generation=%d, want 7", tick.gen)
	}
}

func TestModelFocusSkipDoesNotRecord(t *testing.T) {
	now := time.Date(2026, 5, 13, 10, 2, 0, 0, time.Local)
	m, recorder, _ := newFocusTestModel(now)
	m.prompting = true
	m.promptTarget = time.Date(2026, 5, 13, 10, 0, 0, 0, time.Local)
	m.focusInterval = time.Nanosecond

	updated, _ := m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	mm := updated.(model)
	if mm.prompting {
		t.Fatal("prompt should clear after skip")
	}
	if len(recorder.entries) != 0 {
		t.Fatalf("recorded entries=%d, want 0", len(recorder.entries))
	}
}

func TestModelFocusTickRetargetsOpenPromptToLatestWorkHour(t *testing.T) {
	now := time.Date(2026, 5, 13, 11, 0, 0, 0, time.Local)
	m, _, _ := newFocusTestModel(now)
	oldTarget := time.Date(2026, 5, 13, 10, 0, 0, 0, time.Local)
	m.prompting = true
	m.promptTarget = oldTarget
	m.nextFocusAt = now

	updated, cmd := m.update(focusTickMsg{at: now, gen: m.focusGen})
	mm := updated.(model)

	if !mm.prompting {
		t.Fatal("prompt should remain open")
	}
	if !mm.promptTarget.Equal(now) {
		t.Errorf("promptTarget=%v, want latest target %v", mm.promptTarget, now)
	}
	if cmd == nil {
		t.Fatal("expected notification/reschedule command")
	}
}

func TestFocusIntervalRequiresWorkBlock(t *testing.T) {
	outside := time.Date(2026, 5, 13, 20, 0, 0, 0, time.Local)
	m, _, _ := newFocusTestModel(outside)
	m.focusInterval = time.Minute

	if got, ok := m.nextFocusPromptAt(outside); ok {
		t.Errorf("nextFocusPromptAt outside work block=%v, true; want false", got)
	}

	inside := time.Date(2026, 5, 13, 10, 30, 0, 0, time.Local)
	m.now = func() time.Time { return inside }
	m.recomputeState()
	got, ok := m.nextFocusPromptAt(inside)
	if !ok {
		t.Fatal("nextFocusPromptAt inside work block ok=false, want true")
	}
	want := inside.Add(time.Minute)
	if !got.Equal(want) {
		t.Errorf("nextFocusPromptAt inside work block=%v, want %v", got, want)
	}
}

func TestNotifyCmdIsBestEffort(t *testing.T) {
	now := time.Date(2026, 5, 13, 10, 0, 0, 0, time.Local)
	m, _, notifier := newFocusTestModel(now)
	notifier.err = errors.New("osascript failed")

	msg := m.notifyCmd("title", "body")()
	if msg != nil {
		t.Errorf("notifyCmd msg=%#v, want nil", msg)
	}
	if len(notifier.sent) != 1 {
		t.Fatalf("sent notifications=%d, want 1", len(notifier.sent))
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

type recordingFocusRecorder struct {
	entries []focus.Entry
	err     error
}

func (r *recordingFocusRecorder) Append(entry focus.Entry) error {
	if r.err != nil {
		return r.err
	}
	r.entries = append(r.entries, entry)
	return nil
}

type recordingNotifier struct {
	sent []string
	err  error
}

func (n *recordingNotifier) Send(title, body string) error {
	n.sent = append(n.sent, title+"|"+body)
	return n.err
}

func newFocusTestModel(now time.Time) (model, *recordingFocusRecorder, *recordingNotifier) {
	recorder := &recordingFocusRecorder{}
	notifier := &recordingNotifier{}
	events := []schedule.Event{
		{
			ID:    "work",
			Title: "Work",
			Start: time.Date(2026, 5, 13, 9, 0, 0, 0, time.Local),
			End:   time.Date(2026, 5, 13, 18, 0, 0, 0, time.Local),
		},
	}
	m := newTestModel(now, events)
	m.focusEnabled = true
	m.focusRecorder = recorder
	m.focusNotifier = notifier
	m.focusGen = 1
	m.setNextFocusPromptAt(now)
	return m, recorder, notifier
}

func runBatch(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		msgs := make([]tea.Msg, 0, len(batch))
		for _, batched := range batch {
			if batched != nil {
				msgs = append(msgs, batched())
			}
		}
		return msgs
	}
	return []tea.Msg{msg}
}
