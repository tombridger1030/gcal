package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

// errNoFetcher is the only programming-error case that crosses into the
// command goroutine; surfaced as a fetchErrMsg rather than a panic so the
// UI can render something coherent.
var errNoFetcher = stringErr("ui: model has no fetcher wired")

type stringErr string

func (e stringErr) Error() string { return string(e) }
