package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tombridger1030/gcal/internal/focus"
)

// fetchCmd returns a tea.Cmd that runs the fetcher on a background
// goroutine and emits fetchDoneMsg or fetchErrMsg.
//
// Contract:
//
//	Postcondition: the returned Cmd never panics; it always emits exactly
//	one message.
func (m model) fetchCmd(day time.Time) tea.Cmd {
	fetcher := m.fetcher
	parent := m.ctx
	stale := len(m.events) > 0
	return func() tea.Msg {
		if fetcher == nil {
			return fetchErrMsg{err: errNoFetcher, stale: stale}
		}
		ctx, cancel := context.WithTimeout(parent, fetchTimeout)
		defer cancel()
		events, err := fetcher.FetchDay(ctx, day)
		if err != nil {
			return fetchErrMsg{err: err, stale: stale}
		}
		return fetchDoneMsg{events: events, at: day}
	}
}

// scheduleNextTransition sets a single timer for the next visible change,
// capped at timerCap so macOS sleep can't mask the boundary by more than
// that. The fired timer's actual fire time is delivered in transitionMsg.at
// so the model can detect wake (see wakeDetected).
func (m *model) scheduleNextTransition() tea.Cmd {
	now := m.now()
	target := m.state.NextTransition(now)
	delay := nextTimerDelay(now, target, timerCap)
	scheduled := now.Add(delay)
	m.scheduledAt = scheduled
	return func() tea.Msg {
		if delay > 0 {
			t := time.NewTimer(delay)
			defer t.Stop()
			<-t.C
		}
		return transitionMsg{at: time.Now()}
	}
}

// scheduleFocusPrompt sets a capped timer for the next focus prompt.
func (m *model) scheduleFocusPrompt() tea.Cmd {
	if !m.focusEnabled {
		return nil
	}
	gen := m.focusGen
	now := m.now()
	if m.nextFocusAt.IsZero() {
		if !m.setNextFocusPromptAt(now) {
			return nil
		}
	}
	target := m.nextFocusAt
	delay := nextTimerDelay(now, target, timerCap)
	return func() tea.Msg {
		if delay > 0 {
			t := time.NewTimer(delay)
			defer t.Stop()
			<-t.C
		}
		return focusTickMsg{at: time.Now(), gen: gen}
	}
}

// rescheduleFocusPrompt invalidates any older focus timer chain and arms
// a fresh timer for the current target.
func (m *model) rescheduleFocusPrompt() tea.Cmd {
	if !m.focusEnabled {
		return nil
	}
	m.focusGen++
	return m.scheduleFocusPrompt()
}

// recordFocusCmd appends one focus entry off the update path.
func (m model) recordFocusCmd(entry focus.Entry) tea.Cmd {
	recorder := m.focusRecorder
	return func() tea.Msg {
		if recorder == nil {
			return focusErrMsg{err: stringErr("ui: model has no focus recorder wired")}
		}
		if err := recorder.Append(entry); err != nil {
			return focusErrMsg{err: err}
		}
		return focusLoggedMsg{}
	}
}

// notifyCmd sends a best-effort notification. Errors intentionally do not
// cross back into the state machine; the in-TUI prompt is authoritative.
func (m model) notifyCmd(title, body string) tea.Cmd {
	notifier := m.focusNotifier
	return func() tea.Msg {
		if notifier != nil {
			_ = notifier.Send(title, body)
		}
		return nil
	}
}

// errNoFetcher is the only programming-error case that crosses into the
// command goroutine; surfaced as a fetchErrMsg rather than a panic so the
// UI can render something coherent.
var errNoFetcher = stringErr("ui: model has no fetcher wired")

type stringErr string

func (e stringErr) Error() string { return string(e) }
