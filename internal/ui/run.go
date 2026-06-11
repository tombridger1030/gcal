// Package ui implements the Bubble Tea program that renders today's
// calendar as a vertical column of time-blocks. Callers see one entry
// point: Run. Everything else — the model, message taxonomy, transition
// timer, and rendering — is unexported.
package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tombridger1030/gcal/internal/focus"
	"github.com/tombridger1030/gcal/internal/notify"
)

// FocusOptions configures hourly focus check-ins. A zero value disables the
// feature; production callers pass Enabled with a Recorder and Notifier.
type FocusOptions struct {
	Enabled  bool
	Recorder focus.Recorder
	Notifier notify.Notifier

	// Interval is a testing override. When positive and the current time is
	// inside a work-titled block, prompts are scheduled after this duration
	// instead of the next top-of-hour.
	Interval time.Duration
}

// Options groups optional UI behavior without widening Run's signature for
// every future setting.
type Options struct {
	Focus FocusOptions
}

// Run launches the TUI and blocks until the user quits or ctx is cancelled.
//
// Contract:
//
//	Preconditions:
//	  - ctx and fetcher must be non-nil.
//	  - when opts.Focus.Enabled is true, Recorder and Notifier must be non-nil.
//	Postcondition: the terminal is restored before return; any in-flight
//	fetch is cancelled by ctx and aborts cleanly.
func Run(ctx context.Context, fetcher Fetcher, opts Options) error {
	if ctx == nil {
		panic("ui.Run: ctx is nil")
	}
	if fetcher == nil {
		panic("ui.Run: fetcher is nil")
	}
	if opts.Focus.Enabled {
		if opts.Focus.Recorder == nil {
			panic("ui.Run: focus recorder is nil")
		}
		if opts.Focus.Notifier == nil {
			panic("ui.Run: focus notifier is nil")
		}
		if opts.Focus.Interval < 0 {
			panic("ui.Run: focus interval is negative")
		}
	}

	m := newModel(nil)
	m.ctx = ctx
	m.fetcher = fetcher
	m.focusEnabled = opts.Focus.Enabled
	m.focusRecorder = opts.Focus.Recorder
	m.focusNotifier = opts.Focus.Notifier
	m.focusInterval = opts.Focus.Interval
	if m.focusEnabled {
		m.focusGen = 1
		m.setNextFocusPromptAt(m.now())
	}

	prog := tea.NewProgram(m,
		tea.WithContext(ctx),
		tea.WithAltScreen(),
	)
	_, err := prog.Run()
	return err
}
